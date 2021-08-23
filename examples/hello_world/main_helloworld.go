package main

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"github.com/hwangtou/go-atomos/examples/hello_world/element"
)

func main() {
	runnable := atomos.CosmosRunnable{}
	runnable.AddElementInterface(api.GetTaskBoothInterface(&element.TaskBoothElement{}))
	runnable.AddElementImplementation(api.GetTaskBoothImplement(&element.TaskBoothElement{}))
	runnable.SetScript(scriptHelloWorld)
	config := &atomos.Config{
		Node:               "TaskBooth",
		LogPath:            "/tmp/cosmos_log/",
		LogLevel:           atomos.LogLevel_Debug,
		EnableCert: &atomos.CertConfig{
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
	cosmos.SendRunnable(runnable)
	<-exitCh
}

func scriptHelloWorld(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
	demoTaskBooth(cosmos, mainId)
	<-killNoticeChannel
}

func demoTaskBooth(cosmos *atomos.CosmosSelf, mainId atomos.MainId) {
	// Try to spawn a TaskBooth atom.
	taskBoothId, err := api.SpawnTaskBooth(cosmos.Local(), "Demo", nil)
	if err != nil {
		panic(err)
	}
	// Kill when exit.
	defer func() {
		mainId.Log().Info("Exiting")
		if err := taskBoothId.Kill(mainId); err != nil {
			mainId.Log().Info("Exiting error, err=%v", err)
		}
	}()

	// Start task demo.
	if _, err = taskBoothId.StartTask(mainId, &api.StartTaskReq{}); err != nil {
		panic(err)
	}
}
