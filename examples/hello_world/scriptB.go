package main

import (
	"fmt"
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello_world/api"
)

var runnableB atomos.CosmosRunnable

func init()  {
	runnableB.SetScript(ScriptB)
}

func ScriptB(cosmos *atomos.CosmosSelf, mainId atomos.MainId, killNoticeChannel chan bool) {
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
	remoteCosmos, err := cosmos.Connect("GreeterA", "127.0.0.1:10001")
	if err != nil {
		fmt.Println("connect err", err)
		return
	}
	fmt.Println("connected")
	id, err := api.GetGreeterId(remoteCosmos, "a")
	if err != nil {
		fmt.Println("GetGreeterId err", err)
		return
	}
	reply, err := id.SayHello(mainId, &api.HelloRequest{Name: "Remote"})
	if err != nil {
		fmt.Println("Reply err", err)
	} else {
		fmt.Println("Reply", reply)
	}
	<-killNoticeChannel
}