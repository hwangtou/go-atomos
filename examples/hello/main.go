package main

import (
	"flag"
	atomos "github.com/hwangtou/go-atomos"
)

func main() {
	// Load runnable.
	cosmos, err := atomos.NewCosmosProcess()
	if err != nil {
		panic(err.Message)
		return
	}
	defer cosmos.Close()

	exitCh, err := cosmos.Daemon()
	if err != nil {
		cosmos.Logging(atomos.LogLevel_Fatal, "Daemon failed, err=(%v)", err)
		return
	}

	// Config
	configPath := flag.String("config", "config/config.yaml", "yaml config path")
	flag.Parse()

	// Load config.
	conf, err := atomos.ConfigFromYaml(*configPath)
	if err != nil {
		cosmos.Logging(atomos.LogLevel_Fatal, "Load config error, err=%v", err)
		return
	}
	if err = conf.Check(); err != nil {
		cosmos.Logging(atomos.LogLevel_Fatal, "Check config error, err=%v", err)
		return
	}
	AtomosRunnable.SetConfig(conf)

	if err = cosmos.Send(atomos.NewRunnableCommand(&AtomosRunnable)); err != nil {
		cosmos.Logging(atomos.LogLevel_Fatal, "Run atomos error, err=%v", err)
		return
	}
	cosmos.WaitKillSignal()
	<-exitCh
}
