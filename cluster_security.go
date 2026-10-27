package atomos

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const atomosIdentityScheme = "atomos"

func atomosCertificateIdentity(cosmos, node string) string {
	return (&url.URL{Scheme: atomosIdentityScheme, Host: cosmos, Path: "/" + node}).String()
}

func certificateAtomosIdentity(cert *x509.Certificate) (cosmos, node string, err error) {
	if cert == nil {
		return "", "", fmt.Errorf("missing client certificate")
	}
	found := false
	for _, identity := range cert.URIs {
		if identity == nil || identity.Scheme != atomosIdentityScheme {
			continue
		}
		if found {
			return "", "", fmt.Errorf("client certificate contains multiple Atomos identities")
		}
		found = true
		cosmos, node = identity.Host, strings.TrimPrefix(identity.EscapedPath(), "/")
		if decoded, decodeErr := url.PathUnescape(node); decodeErr == nil {
			node = decoded
		}
		if identity.User != nil || identity.RawQuery != "" || identity.Fragment != "" || !checkCosmosName(cosmos) || !checkNodeName(node) || identity.EscapedPath() != "/"+url.PathEscape(node) {
			return "", "", fmt.Errorf("invalid Atomos identity URI %q", identity)
		}
	}
	if !found {
		return "", "", fmt.Errorf("client certificate has no Atomos identity URI SAN")
	}
	return cosmos, node, nil
}

func validateClientCertificateIdentity(certPEM []byte, cosmos, node string) *Error {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return NewError(ErrCodeCosmosEtcdClusterTLSInvalid, "cluster: failed to parse client certificate").AddStack(nil)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return NewErrorf(ErrCodeCosmosEtcdClusterTLSInvalid, "cluster: failed to parse client certificate: %v", err).AddStack(nil)
	}
	certCosmos, certNode, identityErr := certificateAtomosIdentity(cert)
	if identityErr != nil {
		return NewErrorf(ErrCodeCosmosEtcdClusterTLSInvalid, "cluster: invalid client identity: %v", identityErr).AddStack(nil)
	}
	if certCosmos != cosmos || certNode != node {
		return NewErrorf(ErrCodeCosmosEtcdClusterTLSInvalid, "cluster: client certificate identity %s/%s does not match local node %s/%s", certCosmos, certNode, cosmos, node).AddStack(nil)
	}
	return nil
}

func requestCallerID(req any) (*IDInfo, bool) {
	switch r := req.(type) {
	case *CosmosRemoteSpawnAtomReq:
		return r.CallerId, true
	case *CosmosRemoteSyncMessagingByNameReq:
		return r.CallerId, true
	case *CosmosRemoteAsyncMessagingByNameReq:
		return r.CallerId, true
	case *CosmosRemoteAsyncOnMessageCallbackReq:
		return r.CallerId, true
	case *CosmosRemoteKillAtomReq:
		return r.CallerId, true
	case *CosmosRemoteElementBroadcastReq:
		return r.CallerId, true
	case *CosmosRemoteSpawnAtomControlledReq:
		return r.CallerId, true
	case *CosmosRemoteHaltAtomControlledReq:
		return r.CallerId, true
	case *CosmosRemoteTakeoverAtomReq:
		return r.CallerId, true
	default:
		return nil, false
	}
}

func isRemoteManagementMethod(method string) bool {
	return method == AtomosRemoteService_TryKilling_FullMethodName || method == AtomosRemoteService_DrainNode_FullMethodName
}

func insecurePeerIsLocal(p *CosmosProcess, ctx context.Context) bool {
	if p.cluster.grpcTransport != nil {
		return true
	}
	remotePeer, ok := peer.FromContext(ctx)
	if !ok || remotePeer.Addr == nil {
		return false
	}
	host, _, err := net.SplitHostPort(remotePeer.Addr.String())
	if err != nil {
		host = remotePeer.Addr.String()
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (p *CosmosProcess) authenticationMiddleware() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !p.cluster.mutualTLS {
			if !p.cluster.allowInsecure || !insecurePeerIsLocal(p, ctx) {
				return nil, status.Error(codes.Unauthenticated, "cluster RPC requires mutual TLS node authentication")
			}
			if isRemoteManagementMethod(info.FullMethod) {
				return nil, status.Error(codes.PermissionDenied, "remote node management is disabled in insecure mode")
			}
			return handler(ctx, req)
		}

		remotePeer, ok := peer.FromContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing RPC peer")
		}
		tlsInfo, ok := remotePeer.AuthInfo.(credentials.TLSInfo)
		if !ok || len(tlsInfo.State.PeerCertificates) == 0 {
			return nil, status.Error(codes.Unauthenticated, "cluster RPC requires a verified client certificate")
		}
		cosmos, node, err := certificateAtomosIdentity(tlsInfo.State.PeerCertificates[0])
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid Atomos client certificate identity")
		}
		if cosmos != p.local.GetIDInfo().Cosmos {
			return nil, status.Error(codes.PermissionDenied, "client certificate belongs to another Cosmos")
		}
		if isRemoteManagementMethod(info.FullMethod) {
			if _, allowed := p.cluster.adminNodes[node]; !allowed {
				return nil, status.Error(codes.PermissionDenied, "authenticated node is not a cluster administrator")
			}
		}
		if caller, required := requestCallerID(req); required {
			if caller == nil {
				return nil, status.Error(codes.Unauthenticated, "missing caller identity")
			}
			if caller.Cosmos != cosmos || caller.Node != node {
				return nil, status.Error(codes.PermissionDenied, "caller identity does not match the authenticated node")
			}
		}
		return handler(ctx, req)
	}
}
