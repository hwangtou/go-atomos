package main

import (
	"flag"
	"fmt"
	"plugin"
	"time"

	atomos "github.com/hwangtou/go-atomos"
)

func main() {
	configPath := flag.String("config", "examples/hello/config/config.yaml", "yaml config path")
	runnablePath := flag.String("runnable", "examples/hello/bin/atomos_hello", "atomos runnable file path")
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
	var runNow = *runnablePath != ""
	var runnable *atomos.CosmosRunnable
	if runNow {
		plug, err := plugin.Open(*runnablePath)
		if err != nil {
			atomos.LogWrite(atomos.LogFormatter(time.Now(), atomos.LogLevel_Fatal, fmt.Sprintf("Open plugin error, err=%v", err)), true)
			return
		}
		r, err := plug.Lookup(atomos.RunnableName)
		if err != nil {
			atomos.LogWrite(atomos.LogFormatter(time.Now(), atomos.LogLevel_Fatal, fmt.Sprintf("Load plugin error, err=%v", err)), true)
			return
		}
		var ok bool
		runnable, ok = r.(*atomos.CosmosRunnable)
		if !ok || runnable == nil {
			atomos.LogWrite(atomos.LogFormatter(time.Now(), atomos.LogLevel_Fatal, fmt.Sprintf("Check plugin error, err=type error,type=%T", r)), true)
			return
		}
	}

	cosmos := atomos.NewCosmosCycle()
	defer cosmos.Close()

	exitCh, err := cosmos.Daemon(conf)
	if err != nil {
		atomos.LogWrite(atomos.LogFormatter(time.Now(), atomos.LogLevel_Fatal, fmt.Sprintf("Run atomos error, err=%v", err)), true)
		return
	}
	if runNow {
		if err = cosmos.Send(atomos.NewRunnableCommand(runnable)); err != nil {
			atomos.LogWrite(atomos.LogFormatter(time.Now(), atomos.LogLevel_Fatal, fmt.Sprintf("Run atomos error, err=%v", err)), true)
			return
		}
	}
	cosmos.WaitKillSignal()
	<-exitCh
}
