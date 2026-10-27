//go:build integration

package atomos

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

// TLS tests for getClusterTLSConfig, which reads the cluster's gRPC TLS
// material from etcd. The decision matrix (no TLS / invalid input) is asserted
// directly; the interesting configs are proven with REAL gRPC handshakes —
// a server started with the returned server option and a client dialed with
// the returned dial option — because options that never complete a handshake
// would otherwise look "valid" to a pure unit test.

// ---- certificate helpers ----

type testCert struct {
	certPEM, keyPEM []byte
	cert            *x509.Certificate
	key             *ecdsa.PrivateKey
}

// newTestCert creates a key pair and certificate with loopback SANs.
func newTestCert(t *testing.T, cn string, isCA bool, parent *testCert) *testCert {
	t.Helper()
	return newTestCertWithSAN(t, cn, isCA, parent, true)
}

func newTestCertWithSAN(t *testing.T, cn string, isCA bool, parent *testCert, withSAN bool) *testCert {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("gen serial: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  isCA,
	}
	if withSAN {
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		tmpl.DNSNames = []string{"localhost"}
	}
	issuer, issuerKey := tmpl, key
	if parent != nil {
		issuer, issuerKey = parent.cert, parent.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, issuer, &key.PublicKey, issuerKey)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return &testCert{
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
		cert:    cert,
		key:     key,
	}
}

// ---- TLS material in etcd ----

func putTLSKey(t *testing.T, cli EtcdClient, key string, value []byte) {
	t.Helper()
	if value == nil {
		return
	}
	if err := etcdPut(cli, key, value); err != nil {
		t.Fatalf("put TLS key %s: %v", key, err.Message)
	}
}

func clearTLSCfg(t *testing.T, cli EtcdClient) {
	t.Helper()
	for _, key := range []string{etcdClusterServerCertKey, etcdClusterServerKeyKey, etcdClusterClientCertKey, etcdClusterClientKeyKey} {
		if err := etcdDelete(cli, key); err != nil {
			t.Fatalf("delete TLS key %s: %v", key, err.Message)
		}
	}
}

// ---- real-handshake helpers ----

// startPeerCNServer starts a gRPC server with the given server option and one
// unary method that reports the CommonName of the certificate the CLIENT
// presented (how the server-side view of mutual TLS is made observable).
func startPeerCNServer(t *testing.T, serverOption *grpc.ServerOption) (addr string, call func(dialOption *grpc.DialOption) (string, error)) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	svr := grpc.NewServer(*serverOption)
	svr.RegisterService(&grpc.ServiceDesc{
		ServiceName: "atomos.tlstest",
		Methods: []grpc.MethodDesc{{
			MethodName: "PeerCN",
			Handler: func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
				cn := "<no-tls>"
				if p, ok := peer.FromContext(ctx); ok {
					if tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo); ok {
						if len(tlsInfo.State.PeerCertificates) > 0 {
							cn = tlsInfo.State.PeerCertificates[0].Subject.CommonName
						} else {
							cn = "<no-cert>"
						}
					}
				}
				return structpb.NewStruct(map[string]any{"cn": cn})
			},
		}},
	}, nil)
	go func() {
		_ = svr.Serve(lis)
	}()
	t.Cleanup(svr.Stop)

	addr = lis.Addr().String()
	call = func(dialOption *grpc.DialOption) (string, error) {
		conn, er := grpc.NewClient(addr, *dialOption)
		if er != nil {
			return "", er
		}
		defer func() { _ = conn.Close() }()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		reply := &structpb.Struct{}
		if er := conn.Invoke(ctx, "/atomos.tlstest/PeerCN", &emptypb.Empty{}, reply); er != nil {
			return "", er
		}
		return reply.GetFields()["cn"].GetStringValue(), nil
	}
	return addr, call
}

// newTLSProcess returns a lightweight process for calling getClusterTLSConfig
// (the method only needs the etcd client and error construction, no Start).
func newTLSProcess(t *testing.T) *CosmosProcess {
	t.Helper()
	return newTestCosmosProcessWithoutCluster(t, "tlsCosmos", "tlsNode")
}

func Test_getClusterTLSConfig_NoTLSAndInvalidInputs(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	p := newTLSProcess(t)

	// Nothing in etcd → no TLS, no options.
	clearTLSCfg(t, ee.client)
	isTLS, serverOption, dialOption, err := p.getClusterTLSConfig(ee.client)
	if err != nil || isTLS || serverOption != nil || dialOption != nil {
		t.Fatalf("no keys: isTLS=(%v), serverOption=(%v), dialOption=(%v), err=(%v)", isTLS, serverOption, dialOption, err)
	}

	// Half-configured TLS material must fail loudly (it used to silently
	// degrade: server cert without key → plaintext cluster; client material
	// without the server pair → plaintext; half-written client pair → a
	// mutual-TLS config whose handshake can never succeed).
	server := newTestCert(t, "atomos-test-server", true, nil)
	other := newTestCert(t, "atomos-test-other", true, nil)

	cases := []struct {
		name                             string
		cert, key, clientCert, clientKey []byte
	}{
		{"server cert without key", server.certPEM, nil, nil, nil},
		{"server key without cert", nil, server.keyPEM, nil, nil},
		{"client pair without server pair", nil, nil, server.certPEM, server.keyPEM},
		{"client cert without key", server.certPEM, server.keyPEM, server.certPEM, nil},
		{"client key without cert", server.certPEM, server.keyPEM, nil, server.keyPEM},
		{"garbage server cert", []byte("not a pem"), server.keyPEM, nil, nil},
		{"server cert/key mismatch", server.certPEM, other.keyPEM, nil, nil},
		{"garbage client cert with valid server pair", server.certPEM, server.keyPEM, []byte("not a pem"), server.keyPEM},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearTLSCfg(t, ee.client)
			putTLSKey(t, ee.client, etcdClusterServerCertKey, c.cert)
			putTLSKey(t, ee.client, etcdClusterServerKeyKey, c.key)
			putTLSKey(t, ee.client, etcdClusterClientCertKey, c.clientCert)
			putTLSKey(t, ee.client, etcdClusterClientKeyKey, c.clientKey)

			isTLS, _, _, err := p.getClusterTLSConfig(ee.client)
			if err == nil {
				t.Fatalf("expected ErrCodeCosmosEtcdClusterTLSInvalid, got nil (isTLS=%v)", isTLS)
			}
			if code := CodeOf(err); code != ErrCodeCosmosEtcdClusterTLSInvalid {
				t.Errorf("code=(%d), want ErrCodeCosmosEtcdClusterTLSInvalid; err=(%v)", code, err)
			}
		})
	}
}

func Test_getClusterTLSConfig_ServerOnlyTLSHandshake(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	p := newTLSProcess(t)

	server := newTestCert(t, "atomos-test-server", true, nil)
	clearTLSCfg(t, ee.client)
	putTLSKey(t, ee.client, etcdClusterServerCertKey, server.certPEM)
	putTLSKey(t, ee.client, etcdClusterServerKeyKey, server.keyPEM)

	isTLS, serverOption, dialOption, err := p.getClusterTLSConfig(ee.client)
	if err != nil || !isTLS || serverOption == nil || dialOption == nil {
		t.Fatalf("server-only TLS: isTLS=(%v), err=(%v)", isTLS, err)
	}

	// Server-only TLS still authenticates the CA and checks the loopback SAN.
	_, call := startPeerCNServer(t, serverOption)
	cn, cerr := call(dialOption)
	if cerr != nil {
		t.Fatalf("server-only TLS handshake failed: %v", cerr)
	}
	// No client cert is configured in this mode: the server must see none.
	if cn != "<no-cert>" {
		t.Fatalf("server-only TLS: server saw client CN=(%q), want no client cert", cn)
	}

	other := newTestCert(t, "atomos-untrusted-root", true, nil)
	clearTLSCfg(t, ee.client)
	putTLSKey(t, ee.client, etcdClusterServerCertKey, other.certPEM)
	putTLSKey(t, ee.client, etcdClusterServerKeyKey, other.keyPEM)
	_, _, untrustedDial, err := p.getClusterTLSConfig(ee.client)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(untrustedDial); err == nil {
		t.Fatal("server-only TLS trusted an unrelated CA")
	}
}

func Test_getClusterTLSConfig_ServerOnlyRejectsMissingSAN(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	p := newTLSProcess(t)
	server := newTestCertWithSAN(t, "atomos-test-server", true, nil, false)
	clearTLSCfg(t, ee.client)
	putTLSKey(t, ee.client, etcdClusterServerCertKey, server.certPEM)
	putTLSKey(t, ee.client, etcdClusterServerKeyKey, server.keyPEM)
	_, serverOption, dialOption, err := p.getClusterTLSConfig(ee.client)
	if err != nil {
		t.Fatal(err)
	}
	_, call := startPeerCNServer(t, serverOption)
	if _, err := call(dialOption); err == nil {
		t.Fatal("server-only TLS accepted a certificate without a matching SAN")
	}
}

func Test_getClusterTLSConfigFromFiles_Handshake(t *testing.T) {
	server := newTestCert(t, "atomos-yaml-server", true, nil)
	dir := t.TempDir()
	certPath, keyPath := dir+"/cluster.crt", dir+"/cluster.key"
	if err := os.WriteFile(certPath, server.certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, server.keyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	isTLS, serverOption, dialOption, err := getClusterTLSConfigFromFiles(&CertConfig{CertPath: certPath, KeyPath: keyPath})
	if err != nil || !isTLS {
		t.Fatalf("file TLS config: isTLS=(%v), err=(%v)", isTLS, err)
	}
	_, call := startPeerCNServer(t, serverOption)
	if _, err := call(dialOption); err != nil {
		t.Fatalf("file TLS handshake failed: %v", err)
	}
}

func Test_getClusterTLSConfig_MutualTLSHandshake(t *testing.T) {
	ee := newEmbeddedEtcd(t)
	p := newTLSProcess(t)

	// Model 1 — shared cluster certificate: the same self-signed pair is used
	// as server identity and client identity. Both sides verify against a pool
	// containing that one certificate.
	shared := newTestCert(t, "atomos-test-shared", true, nil)
	clearTLSCfg(t, ee.client)
	putTLSKey(t, ee.client, etcdClusterServerCertKey, shared.certPEM)
	putTLSKey(t, ee.client, etcdClusterServerKeyKey, shared.keyPEM)
	putTLSKey(t, ee.client, etcdClusterClientCertKey, shared.certPEM)
	putTLSKey(t, ee.client, etcdClusterClientKeyKey, shared.keyPEM)

	isTLS, serverOption, dialOption, err := p.getClusterTLSConfig(ee.client)
	if err != nil || !isTLS || serverOption == nil || dialOption == nil {
		t.Fatalf("mutual TLS (shared cert): isTLS=(%v), err=(%v)", isTLS, err)
	}
	_, call := startPeerCNServer(t, serverOption)
	cn, cerr := call(dialOption)
	if cerr != nil {
		t.Fatalf("mutual TLS (shared cert) handshake failed: %v", cerr)
	}
	if cn != "atomos-test-shared" {
		t.Fatalf("mutual TLS (shared cert): server saw client CN=(%q), want atomos-test-shared", cn)
	}

	// Model 2 — CA model: the client material in etcd is a self-signed CA
	// (cert + key); the server presents a certificate signed by that CA.
	// The client must present the CA pair as ITS identity. Regression for the
	// bug where the client certificate was loaded from the SERVER pair: then
	// every client presented the server's identity and client authentication
	// was meaningless — the peer-CN assertion below is what fails pre-fix.
	ca := newTestCert(t, "atomos-test-ca", true, nil)
	signed := newTestCert(t, "atomos-test-server", false, ca)
	clearTLSCfg(t, ee.client)
	putTLSKey(t, ee.client, etcdClusterServerCertKey, signed.certPEM)
	putTLSKey(t, ee.client, etcdClusterServerKeyKey, signed.keyPEM)
	putTLSKey(t, ee.client, etcdClusterClientCertKey, ca.certPEM)
	putTLSKey(t, ee.client, etcdClusterClientKeyKey, ca.keyPEM)

	isTLS, serverOption, dialOption, err = p.getClusterTLSConfig(ee.client)
	if err != nil || !isTLS || serverOption == nil || dialOption == nil {
		t.Fatalf("mutual TLS (CA model): isTLS=(%v), err=(%v)", isTLS, err)
	}
	_, call = startPeerCNServer(t, serverOption)
	cn, cerr = call(dialOption)
	if cerr != nil {
		t.Fatalf("mutual TLS (CA model) handshake failed: %v", cerr)
	}
	if cn != "atomos-test-ca" {
		t.Fatalf("mutual TLS (CA model): server saw client CN=(%q), want atomos-test-ca "+
			"(the client must present the CLIENT pair, not the server pair)", cn)
	}
}
