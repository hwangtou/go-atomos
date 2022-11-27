package main

import (
	"flag"
	atomos "github.com/hwangtou/go-atomos"
	"log"
	"time"
)

func Main(runnable atomos.CosmosRunnable) *atomos.Error {
	log.Println("Welcome to Atomos App!")
	defer log.Println("Bye!")

	var (
		configPath = flag.String("config", "", "config path")
	)

	flag.Parse()

	daemon := atomos.NewCosmosDaemon(*configPath)
	if err := daemon.Check(); err != nil {
		return err.AutoStack(nil, nil)
	}
	if err := daemon.Daemon(); err != nil {
		return err.AutoStack(nil, nil)
	}
	defer daemon.Exit()

	time.Sleep(3600 * time.Second)
	//atomos.CosmosProcessMainFn(runnable)
	return nil
}

func main() {
	var AtomosRunnable atomos.CosmosRunnable
	AtomosRunnable.SetScript(func(main *atomos.CosmosMain, killSignal chan bool) {})
	Main(AtomosRunnable)
}
