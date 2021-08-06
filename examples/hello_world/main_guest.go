package main

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
	"github.com/hwangtou/go-atomos/examples/hello_world/element"
)

func main() {
	runnable := atomos.CosmosRunnable{}
	runnable.AddElementInterface(api.GetGreeterInterface(&element.GreeterElement{}))
	runnable.AddElementImplementation(api.GetGreeterImplement(&element.GreeterElement{}))
	runnable.SetScript(scriptGreeter)
	config := &atomos.Config{
		Node:               "Greeter",
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

func scriptGreeter(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
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
		panic(err)
	} else {
		mainId.Log().Info("Hello, %v", hello)
	}
	mainId.Log().Info("Connecting")
	remoteCosmos, err := cosmos.Connect("GreetingRoom", "127.0.0.1:10001")
	if err != nil {
		mainId.Log().Error("Connect failed, err=%v", err)
		return
	}
	mainId.Log().Info("Connected, remoteCosmos=%+v", remoteCosmos)
	id, err := api.GetGreeterId(remoteCosmos, "a")
	if err != nil {
		mainId.Log().Error("GetGreeterId failed, err=%v", err)
		return
	}
	mainId.Log().Info("GetId, id=%+v", id)
	reply, err := id.SayHello(mainId, &api.HelloRequest{Name: "Remote"})
	if err != nil {
		mainId.Log().Info("Reply, err=%v", err)
	} else {
		mainId.Log().Info("Reply, reply=%v", reply)
	}
	<-killNoticeChannel
}
