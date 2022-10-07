package main

import (
	atomos "github.com/hwangtou/go-atomos"
	"remote/server/api"
	"remote/server/elements"
)

var AtomosRunnable atomos.CosmosRunnable

func init() {
	AtomosRunnable.
		AddElementImplementation(api.GetHelloImplement(&elements.Hello{})).
		SetScript(helloScript)
}

func helloScript(self *atomos.CosmosMain, killCh chan bool) {

	// TODO: BUG，如果是外部Kill的情况，运行到这里就能正常退出。但如果是脚本执行完的情况，就会出现不退出，也KIll不了的问题。
	<-killCh // TODO: Think about error exit, block
}
