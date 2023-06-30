package main

import (
	atomos "github.com/hwangtou/go-atomos"
	"hello_atomos/api"
	"hello_atomos/hello"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	accessLog := func(s string) { os.Stdin.WriteString(s) }
	errLog := func(s string) { os.Stdin.WriteString(s) }
	atomos.InitCosmosProcess("testCosmos", "testCosmosNode", accessLog, errLog)
	if err := atomos.SharedCosmosProcess().Start(&AtomosRunnable); err != nil {
		atomos.SharedCosmosProcess().Self().Log().Error("Start failed. err=(%v)", err)
		return
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh

	if err := atomos.SharedCosmosProcess().Stop(); err != nil {
		atomos.SharedCosmosProcess().Self().Log().Error("Stop failed. err=(%v)", err)
	}
	<-time.After(1 * time.Second)
}

var AtomosRunnable atomos.CosmosRunnable

func init() {
	AtomosRunnable.
		SetConfig(&atomos.Config{
			Node:      "testNode",
			LogPath:   "/tmp/atomos_test.log",
			LogLevel:  0,
			Customize: nil,
		}).
		AddElementImplementation(api.GetHelloAtomosImplement(hello.NewDev())).
		SetMainScript(&hellMain{})
}

type hellMain struct{}

func (m *hellMain) OnStartUp(local *atomos.CosmosProcess) *atomos.Error {
	//TODO implement me
	panic("implement me")
}

func (m *hellMain) OnShutdown() *atomos.Error {
	//TODO implement me
	panic("implement me")
}
