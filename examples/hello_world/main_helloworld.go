package main

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"github.com/hwangtou/go-atomos/examples/hello_world/element"
	"google.golang.org/protobuf/types/known/anypb"
)

func main() {
	// Config
	wh, _ := anypb.New(&api.WormholeBoothSpawnArg{
		Addr: "0.0.0.0:20000",
	})
	// Load runnable
	runnable := atomos.CosmosRunnable{}
	// TaskBooth
	runnable.AddElementImplementation(api.GetTaskBoothImplement(&element.TaskBoothElement{}))
	// RemoteBooth
	runnable.AddElementInterface(api.GetRemoteBoothInterface(&element.RemoteBoothElement{}))
	// LocalBooth
	runnable.AddElementImplementation(api.GetLocalBoothImplement(&element.LocalBoothElement{}))
	// WormholeBooth
	runnable.AddWormhole(api.GetWormholeBoothImplement(&element.WormholeBoothElement{}))
	runnable.SetScript(scriptHelloWorld)
	config := &atomos.Config{
		Node:     api.NodeHelloWorld,
		LogPath:  "/tmp/cosmos_log/",
		LogLevel: atomos.LogLevel_Debug,
		EnableCert: &atomos.CertConfig{
			CertPath:           api.CertPath,
			KeyPath:            api.KeyPath,
			InsecureSkipVerify: true,
		},
		EnableServer: &atomos.RemoteServerConfig{
			Host: api.NodeHost,
			Port: api.NodeHelloPort,
		},
		Customize: map[string]*anypb.Any{
			"Wormhole": wh,
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
	// Exit
	<-exitCh
}

func scriptHelloWorld(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
	//demoTaskBooth(cosmos, mainId, killNoticeChannel)
	//demoRemoteBoothLocal(cosmos, mainId, killNoticeChannel)
	demoWormholeBooth(cosmos, mainId, killNoticeChannel)
}

// TaskBooth
func demoTaskBooth(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
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
	<-killNoticeChannel
}

// RemoteBooth
func demoRemoteBoothLocal(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
	// Try to connect to remote node.
	_, err := cosmos.Connect(api.RemoteName, api.NodeRemoteAddr)
	if err != nil {
		panic(err)
	}
	// Spawn LocalBooth atom.
	_, err = api.SpawnLocalBooth(cosmos.Local(), api.LocalBoothMainAtomName, nil)
	if err != nil {
		panic(err)
	}
	<-killNoticeChannel
}

func demoWormholeBooth(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
	<-killNoticeChannel
}
