package main

import (
	atomos "github.com/hwangtou/go-atomos"

	"github.com/hwangtou/go-atomos/examples/hello/api"
	"github.com/hwangtou/go-atomos/examples/hello/elements"
)

var AtomosRunnable atomos.CosmosRunnable

func init() {
	AtomosRunnable.
		AddElementImplementation(api.GetHelloImplement(&elements.Hello{})).
		SetScript(helloScript)
}

func helloScript(self *atomos.CosmosMain, killCh chan bool) {
	// Get Element
	helloElementId, err := api.GetHelloElementID(self.Cosmos())
	if err != nil {
		self.Log().Error("Get element failed, err=(%v)", err)
		return
	}
	self.Log().Info("Get element succeed, id=(%v)", helloElementId.GetIDInfo())
	// Send Element
	helloResp, err := helloElementId.SayHello(self, &api.HelloReq{Name: "Atomos"})
	if err != nil {
		self.Log().Error("SayHello failed, err=(%v)", err)
		return
	}
	self.Log().Info("Main reply, rsp=(%+v)", helloResp)
	// Spawn
	helloId, err := api.SpawnHelloAtom(self.Cosmos(), "hello", nil)
	if err != nil {
		self.Log().Error("Spawn failed, err=(%v)", err)
		return
	}
	self.Log().Info("Spawn succeed, id=(%v)", helloId.GetIDInfo())
	// Get
	helloId, err = api.GetHelloAtomID(self.Cosmos(), "hello")
	if err != nil {
		self.Log().Error("Get failed, err=(%v)", err)
		return
	}
	self.Log().Info("Get succeed, id=(%v)", helloId.GetIDInfo())
	// Send
	helloResp, err = helloId.SayHello(self, &api.HelloReq{Name: "Atomos"})
	if err != nil {
		self.Log().Error("SayHello failed, err=(%v)", err)
		return
	}
	self.Log().Info("Main reply, rsp=(%+v)", helloResp)
	if _, err = helloId.BuildNet(self, &api.BuildNetReq{Id: 0}); err != nil {
		self.Log().Error("BuildNet failed, err=(%v)", err.AutoStack(self, nil))
		return
	}

	// Panic
	if _, err := helloId.MakePanic(self, &api.MakePanicIn{}); err != nil {
		self.Log().Error("Make panic, err=(%v)", err)
		return
	}
	<-killCh // TODO: Think about error exit, block
}
