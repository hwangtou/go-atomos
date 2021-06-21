package main

import (
	atomos "github.com/hwangtou/go-atomos"
)

func main() {
	config := &atomos.Config{
		Node:    "hello_world",
		LogPath: "/tmp/cosmos_log/",
		LogLevel: atomos.LogLevel_Debug,
	}
	// Cycle
	cosmos := atomos.NewCosmosCycle()
	defer cosmos.Close()
	err := cosmos.DaemonWithRunnable(config, runnable)
	if err != nil {
		panic(err)
	}
}
