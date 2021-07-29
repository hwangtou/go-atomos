package main

import (
	atomos "github.com/hwangtou/go-atomos"
)

func main() {
	go cosmosNodeA()
	cosmosNodeB()
}

func cosmosNodeA() {
	config := &atomos.Config{
		Node:               "GreeterA",
		LogPath:            "/tmp/cosmos_log/",
		LogLevel:           atomos.LogLevel_Debug,
		EnableRemoteServer: &atomos.RemoteServerConfig{
			Port:     10001,
			CertPath: "server.crt",
			KeyPath:  "server.key",
		},
	}
	// Cycle
	cosmos := atomos.NewCosmosCycle()
	defer cosmos.Close()
	exitCh, err := cosmos.Daemon(config)
	if err != nil {
		return
	}
	cosmos.SendRunnable(runnableA)
	<-exitCh
}

func cosmosNodeB() {
	config := &atomos.Config{
		Node:               "GreeterB",
		LogPath:            "/tmp/cosmos_log/",
		LogLevel:           atomos.LogLevel_Debug,
		EnableRemoteServer: &atomos.RemoteServerConfig{
			Port:     10002,
			CertPath: "server.crt",
			KeyPath:  "server.key",
		},
	}
	// Cycle
	cosmos := atomos.NewCosmosCycle()
	defer cosmos.Close()
	exitCh, err := cosmos.Daemon(config)
	if err != nil {
		return
	}
	cosmos.SendRunnable(runnableB)
	<-exitCh
}
