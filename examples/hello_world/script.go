package main

import (
	"fmt"
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
)

var runnable atomos.CosmosRunnable

func init()  {
	runnable.SetScript(Script)
}

func Script(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
	startScript(cosmos, mainId)
}

func startScript(cosmos *atomos.CosmosSelf, mainId atomos.MainId) {
	greeterId, err := api.SpawnGreeter(cosmos.Local(), "a", nil)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := greeterId.Kill(mainId); err != nil {
		}
	}()
	hello, err := greeterId.SayHello(mainId, &api.HelloRequest{Name: "Atomos"})
	if err != nil {
		panic(err)
	} else {
		//// Try to make a crash
		//fmt.Println(hello.Message)
		// todo: use mainId to log.
		fmt.Printf("%v\n", hello)
	}
}
