package main

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"github.com/hwangtou/go-atomos/examples/hello_world/element"
)

func main() {
	runnable := atomos.CosmosRunnable{}
	// RemoteBooth
	runnable.AddElementInterface(api.GetRemoteBoothInterface(&element.RemoteBoothElement{}))
	runnable.AddElementImplementation(api.GetRemoteBoothImplement(&element.RemoteBoothElement{}))
	runnable.AddElementInterface(api.GetLocalBoothInterface(&element.LocalBoothElement{}))
	runnable.SetScript(scriptRemote)
	config := &atomos.Config{
		Node:     api.RemoteName,
		LogPath:  "/tmp/cosmos_log/",
		LogLevel: atomos.LogLevel_Debug,
		EnableCert: &atomos.CertConfig{
			CertPath: api.CertPath,
			KeyPath:  api.KeyPath,
			InsecureSkipVerify: true,
		},
		EnableServer: &atomos.RemoteServerConfig{
			Host: api.NodeHost,
			Port: api.NodeRemotePort,
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

func scriptRemote(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
	demoRemoteBooth(cosmos, mainId, killNoticeChannel)
}

// RemoteBooth
func demoRemoteBooth(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
	// Try to spawn a RemoteBooth atom.
	remoteBoothId, err := api.SpawnRemoteBooth(cosmos.Local(), api.RemoteBoothMainAtomName, nil)
	if err != nil {
		panic(err)
	}
	// Kill when exit.
	defer func() {
		mainId.Log().Info("Exiting")
		if err := remoteBoothId.Kill(mainId); err != nil {
			mainId.Log().Info("Exiting error, err=%v", err)
		}
	}()

	// Start task demo.
	if _, err = remoteBoothId.SayHello(mainId, &api.RemoteSayHelloReq{
		Id:      0,
		Message: "Say Hello to self.",
	}); err != nil {
		panic(err)
	}
	<-killNoticeChannel
}
