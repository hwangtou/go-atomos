package atomos

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

type securityTestCertificate struct {
	certPEM []byte
	keyPEM  []byte
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
}

func newSecurityTestCA(t *testing.T) *securityTestCertificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "atomos-security-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &securityTestCertificate{certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), cert: cert, key: key}
}

func newSecurityTestLeaf(t *testing.T, ca *securityTestCertificate, cosmos, node string, server bool) *securityTestCertificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: node},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	if server {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
		identity, err := url.Parse(atomosCertificateIdentity(cosmos, node))
		if err != nil {
			t.Fatal(err)
		}
		template.URIs = []*url.URL{identity}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return &securityTestCertificate{
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
	}
}

func writeSecurityTestTLSConfig(t *testing.T, ca, server, client *securityTestCertificate) *CertConfig {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	return &CertConfig{
		CertPath:       write("server-chain.crt", append(append([]byte(nil), server.certPEM...), ca.certPEM...)),
		KeyPath:        write("server.key", server.keyPEM),
		ClientCertPath: write("client.crt", client.certPEM),
		ClientKeyPath:  write("client.key", client.keyPEM),
		ClientCaPath:   write("client-ca.crt", ca.certPEM),
	}
}

func TestClusterRPCRejectsUnauthenticatedAndSpoofedCalls(t *testing.T) {
	const cosmos = "securityCosmos"
	process := newTestCosmosProcessAsClusterNode(t, cosmos, "target")
	ca := newSecurityTestCA(t)
	serverCert := newSecurityTestLeaf(t, ca, cosmos, "target", true)
	clientCert := newSecurityTestLeaf(t, ca, cosmos, "target", false)
	config := writeSecurityTestTLSConfig(t, ca, serverCert, clientCert)
	_, serverOption, dialOption, tlsErr := getClusterTLSConfigFromFiles(config)
	if tlsErr != nil {
		t.Fatal(tlsErr)
	}
	if err := validateClientCertificateIdentity(clientCert.certPEM, cosmos, "target"); err != nil {
		t.Fatal(err)
	}
	process.cluster.mutualTLS = true
	process.cluster.adminNodes = map[string]struct{}{}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(*serverOption, grpc.ChainUnaryInterceptor(process.recoveryMiddleware(), process.authenticationMiddleware()))
	RegisterAtomosRemoteServiceServer(server, &atomosRemoteService{process: process})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	address := listener.Addr().String()

	t.Run("plaintext rejected", func(t *testing.T) {
		conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err = NewAtomosRemoteServiceClient(conn).TryKilling(ctx, &CosmosRemoteTryKillingReq{})
		if err == nil {
			t.Fatal("plaintext TryKilling unexpectedly succeeded")
		}
	})

	t.Run("TLS without client certificate rejected", func(t *testing.T) {
		roots := x509.NewCertPool()
		roots.AppendCertsFromPEM(ca.certPEM)
		conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12})))
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err = NewAtomosRemoteServiceClient(conn).TryKilling(ctx, &CosmosRemoteTryKillingReq{})
		if err == nil {
			t.Fatal("TLS client without a certificate unexpectedly succeeded")
		}
	})

	conn, err := grpc.NewClient(address, *dialOption)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := NewAtomosRemoteServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.GetElementInfo(ctx, &CosmosRemoteGetElementInfoReq{Element: ForTestAtomosName}); err != nil {
		t.Fatalf("valid mTLS request rejected: %v", err)
	}

	_, err = client.SpawnAtom(ctx, &CosmosRemoteSpawnAtomReq{
		CallerId: &IDInfo{Type: IDType_Cosmos, Cosmos: cosmos, Node: "spoofed"},
		Element:  ForTestAtomosName,
		Atom:     "attacker",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("spoofed CallerId error = %v, want PermissionDenied", err)
	}
	_, err = client.TryKilling(ctx, &CosmosRemoteTryKillingReq{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("non-admin TryKilling error = %v, want PermissionDenied", err)
	}
	if !process.IsRunning() {
		t.Fatal("non-admin TryKilling changed process state")
	}

	process.cluster.adminNodes["target"] = struct{}{}
	rsp, err := client.TryKilling(ctx, &CosmosRemoteTryKillingReq{})
	if err != nil || rsp.Error != nil {
		t.Fatalf("admin TryKilling rejected: rsp=%v err=%v", rsp, err)
	}
}

func TestInsecureClusterRPCDisablesRemoteManagement(t *testing.T) {
	process := newTestCosmosProcessAsClusterNode(t, "insecureSecurityCosmos", "local")
	process.cluster.allowInsecure = true
	process.cluster.mutualTLS = false
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.ChainUnaryInterceptor(process.recoveryMiddleware(), process.authenticationMiddleware()))
	RegisterAtomosRemoteServiceServer(server, &atomosRemoteService{process: process})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := NewAtomosRemoteServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := client.GetElementInfo(ctx, &CosmosRemoteGetElementInfoReq{Element: ForTestAtomosName}); err != nil {
		t.Fatalf("local insecure test RPC rejected: %v", err)
	}
	if _, err := client.TryKilling(ctx, &CosmosRemoteTryKillingReq{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("insecure TryKilling error = %v, want PermissionDenied", err)
	}
	if !process.IsRunning() {
		t.Fatal("insecure TryKilling changed process state")
	}
}

func TestRunPathSymlinkDoesNotTruncateTarget(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, "run")
	if err := os.Mkdir(runDir, 0700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(dir, "victim")
	original := []byte("do not destroy")
	if err := os.WriteFile(victim, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(runDir, "test")); err != nil {
		t.Fatal(err)
	}
	env := &appEnv{config: &Config{RunPath: runDir}}
	if err := env.checkRunPath(); err != nil {
		t.Fatalf("checkRunPath failed: %v", err)
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("victim changed to %q", got)
	}
}

func TestRecoveryMiddlewareDoesNotLeakRequestOrPanic(t *testing.T) {
	var process *CosmosProcess
	interceptor := process.recoveryMiddleware()
	_, err := interceptor(context.Background(), &String{S: "secret-request-value"}, &grpc.UnaryServerInfo{FullMethod: "/security/Panic"}, func(context.Context, any) (any, error) {
		panic("secret-panic-value")
	})
	if err == nil {
		t.Fatal("panic was not converted to an error")
	}
	if strings.Contains(err.Error(), "secret-request-value") || strings.Contains(err.Error(), "secret-panic-value") {
		t.Fatalf("remote error leaked sensitive data: %v", err)
	}
}
