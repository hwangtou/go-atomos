package main

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"github.com/hwangtou/go-atomos/examples/hello_world/element"
)

func main() {
	runnable := atomos.CosmosRunnable{}
	//runnable.AddElementInterface(api.GetGreeterInterface(&element.GreeterElement{}))
	runnable.AddElementImplementation(api.GetGreeterImplement(&element.GreeterElement{}))
	runnable.SetScript(scriptGreetingRoom)
	config := &atomos.Config{
		Node:               "GreetingRoom",
		LogPath:            "/tmp/cosmos_log/",
		LogLevel:           atomos.LogLevel_Debug,
		EnableServer: &atomos.RemoteServerConfig{
			Port:     10001,
		},
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

func scriptGreetingRoom(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
	greeterId, err := api.SpawnGreeter(cosmos.Local(), "a", nil)
	if err != nil {
		panic(err)
	}
	defer func() {
		mainId.Log().Info("Exiting")
		if err := greeterId.Kill(mainId); err != nil {
		}
	}()
	hello, err := greeterId.SayHello(mainId, &api.HelloRequest{Name: "Atomos"})
	if err != nil {
		mainId.Log().Error("Hello, hello=%v,err=%v", hello, err)
		panic(err)
	} else {
		mainId.Log().Info("Hello, %v", hello)
	}
	<-killNoticeChannel
}
