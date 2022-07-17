package main

import (
	"flag"
	"fmt"
	"time"

	atomos "github.com/hwangtou/go-atomos"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "yaml config path")
	flag.Parse()

	// Load config.
	conf, err := atomos.ConfigFromYaml(*configPath)
	if err != nil {
		atomos.LogWrite(atomos.LogFormatter(time.Now(), atomos.LogLevel_Fatal, fmt.Sprintf("Load config error, err=%v", err)), true)
		return
	}
	if err = conf.Check(); err != nil {
		atomos.LogWrite(atomos.LogFormatter(time.Now(), atomos.LogLevel_Fatal, fmt.Sprintf("Check config error, err=%v", err)), true)
		return
	}

	// Load runnable.
	cosmos := atomos.NewCosmosCycle()
	defer cosmos.Close()

	exitCh, err := cosmos.Daemon(conf)
	if err != nil {
		atomos.LogWrite(atomos.LogFormatter(time.Now(), atomos.LogLevel_Fatal, fmt.Sprintf("Run atomos error, err=%v", err)), true)
		return
	}
	if err = cosmos.Send(atomos.NewRunnableCommand(&AtomosRunnable)); err != nil {
		atomos.LogWrite(atomos.LogFormatter(time.Now(), atomos.LogLevel_Fatal, fmt.Sprintf("Run atomos error, err=%v", err)), true)
		return
	}
	cosmos.WaitKillSignal()
	<-exitCh
}
