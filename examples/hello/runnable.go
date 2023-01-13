package main

import (
	atomos "github.com/hwangtou/go-atomos"
	"github.com/hwangtou/go-atomos/examples/hello/api"
	"github.com/hwangtou/go-atomos/examples/hello/elements"
	"time"
)

var AtomosRunnable atomos.CosmosRunnable

func init() {
	AtomosRunnable.
		setConfig(&atomos.Config{
			Node:         "testNode",
			LogPath:      "/tmp/atomos_test.log",
			LogLevel:     0,
			EnableCert:   nil,
			EnableServer: nil,
			EnableTelnet: nil,
			Customize:    nil,
		}).
		SetMainScript(&hellMainScript{}).
		AddElementImplementation(api.GetHelloImplement(&elements.Hello{}))
}

type hellMainScript struct{}

func (h *hellMainScript) OnStartup() *atomos.Error {
	self := atomos.SharedCosmosProcess().Self()
	// Get Element
	helloElementID, err := api.GetHelloElementID(self.Cosmos())
	if err != nil {
		return err.AddStack(self)
	}
	self.Log().Info("Get element succeed, id=(%v)", helloElementID.GetIDInfo())

	// Send Element
	helloResp, err := helloElementID.SayHello(self, &api.HelloReq{Name: "Atomos"})
	if err != nil {
		return err.AddStack(self)
	}
	self.Log().Info("Main reply, rsp=(%+v)", helloResp)

	// Spawn
	helloID, err := api.SpawnHelloAtom(self.Cosmos(), "hello", nil)
	if err != nil {
		return err.AddStack(self)
	}
	self.Log().Info("Spawn succeed, id=(%v)", helloID.GetIDInfo())

	// Get
	helloID, err = api.GetHelloAtomID(self.Cosmos(), "hello")
	if err != nil {
		return err.AddStack(self)
	}
	self.Log().Info("Get succeed, id=(%v)", helloID.GetIDInfo())

	// Send
	helloResp, err = helloID.SayHello(self, &api.HelloReq{Name: "Atomos"})
	if err != nil {
		return err.AddStack(self)
	}
	self.Log().Info("Main reply, rsp=(%+v)", helloResp)
	if _, err = helloID.BuildNet(self, &api.BuildNetReq{Id: 0}); err != nil {
		return err.AddStack(self)
	}

	// Panic
	if _, err := helloID.MakePanic(self, &api.MakePanicIn{}); err != nil {
		self.Log().Info("Make panic, err=(%v)", err)
		//return
	}

	// Scale 100K
	scaleTimes := 10 //100000
	for i := 0; i < scaleTimes; i += 1 {
		time.Sleep(1 * time.Microsecond)
		go func() {
			_, err = helloElementID.ScaleBonjour(self, &api.BonjourReq{})
			if err != nil {
				self.Log().Error("SayHello failed, err=(%v)", err)
				return
			}
		}()
	}
	// RAM USAGE
	// 1000(315)     -> 11.0MB - 5.2MB   => 5.8MB/315=18K
	// 10000(6614)   -> 95.9MB - 18.5MB  => 77.4MB/6614=11K
	// 100000(11710) -> 562.1MB - 79.3MB => 482.8MB/11710=41K
	return nil
}

func (h *hellMainScript) OnShutdown() *atomos.Error {
	return nil
}
