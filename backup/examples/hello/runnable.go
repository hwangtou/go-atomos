package main

import (
	atomos "github.com/hwangtou/go-atomos"

	"github.com/hwangtou/go-atomos/examples/hello/api"
	"github.com/hwangtou/go-atomos/examples/hello/elements"
)

var AtomosRunnable atomos.CosmosRunnable

func init() {
	AtomosRunnable.
		AddElementImplementation(api.GetHelloImplement(&elements.HelloElement{})).
		SetScript(helloScript)
}

func helloScript(self *atomos.CosmosSelf, id atomos.MainId, channel chan bool) {
	helloId, err := api.SpawnHello(self.Local(), "hello", nil)
	if err != nil {
		id.Log().Error("Spawn failed, err=%v", err)
		return
	}
	helloResp, err := helloId.SayHello(id, &api.HelloReq{Name: "Atomos"})
	if err != nil {
		id.Log().Error("SayHello failed, err=%v", err)
		return
	}
	id.Log().Info("Main reply, %+v", helloResp)
	if _, err = helloId.BuildNet(id, &api.BuildNetReq{Id: 0}); err != nil {
		id.Log().Error("BuildNet failed, err=%v", err)
		return
	}
	<-channel // TODO: Think about error exit, block
}
