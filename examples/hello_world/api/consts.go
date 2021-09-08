package api

import "fmt"

const (
	// Cert
	CertPath = "server.crt"
	KeyPath = "server.key"
	// Remote
	RemoteName              = "Remote"
	NodeHost                = ""
	NodeRemotePort          = 10000
	NodeHelloPort           = 10001
	RemoteBoothMainAtomName = "RemoteBooth"
	// Local
	NodeHelloWorld = "Hello"
	LocalBoothMainAtomName = "LocalBooth"
)

var (
	NodeRemoteAddr = fmt.Sprintf("%s:%d", NodeHost, NodeRemotePort)
)
