package main

import (
	"fmt"
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
)

var runnableA atomos.CosmosRunnable

func init()  {
	runnableA.SetScript(ScriptA)
}

func ScriptA(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
	greeterId, err := api.SpawnGreeter(cosmos.Local(), "a", nil)
	if err != nil {
		panic(err)
	}
	defer func() {
		fmt.Println("script exit")
		if err := greeterId.Kill(mainId); err != nil {
		}
	}()
	hello, err := greeterId.SayHello(mainId, &api.HelloRequest{Name: "Atomos"})
	if err != nil {
		panic(err)
	} else {
		// todo: use mainId to log.
		fmt.Println("script", hello)
	}
	<-killNoticeChannel
}
