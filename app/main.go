package main

import (
	"flag"
	"fmt"
	atomos "github.com/hwangtou/go-atomos"
	"log"
	"os"
)

func Main(runnable atomos.CosmosRunnable) *atomos.Error {
	log.Println("Welcome to Atomos Supervisor!", os.Getpid())
	defer log.Println("Bye!")

	var (
		configPath = flag.String("config", "", "config path")
	)

	flag.Parse()

	var err *atomos.Error
	conf, err = loadConfig(*configPath)
	if err != nil {
		err = err.AutoStack(nil, nil)
		log.Printf("Daemon process load config failed, config=(%s),err=(%v)", *configPath, err)
		return
	}

	// Open Log.
	logFile, err := atomos.NewLogFile(conf.LogPath)
	if err != nil {
		err = err.AutoStack(nil, nil)
		log.Printf("Daemon process load log file failed, config=(%s),err=(%v)", *configPath, err)
		return
	}
	defer logFile.Close()

	cmd := &command{configPath: *configPath}
	daemon := atomos.NewCosmosDaemon(conf, logFile, cmd)
	cmd.daemon = daemon
	hasRun, err := daemon.Check()
	if err != nil && !hasRun {
		logFile.WriteAccessLog(fmt.Sprintf("Daemon process check failed, err=(%v)\n", err))
		return
	}
	if hasRun {
		logFile.WriteAccessLog(fmt.Sprintf("Daemon process has run, err=(%v)\n", err))
		return
	}
	isDaemon, daemonPid, err := daemon.CreateProcess()
	if err != nil {
		logFile.WriteAccessLog(fmt.Sprintf("Daemon process create process failed, err=(%v)\n", err))
		return
	}
	if !isDaemon {
		logFile.WriteAccessLog(fmt.Sprintf("Daemon process launcher is exiting, launchedPid=(%d)\n", daemonPid))
		os.Exit(0)
	}
	if err := daemon.Daemon(); err != nil {
		logFile.WriteAccessLog(fmt.Sprintf("Daemon process failed, err=(%v)\n", err))
		return
	}

	logFile.WriteAccessLog(fmt.Sprintf("Daemon process is running, pid=(%d)\n", os.Getpid()))

	nodeList, err = cmd.getNodeListConfig()
	if err != nil {
		logFile.WriteErrorLog(fmt.Sprintf("Daemon process get node list config failed, err=(%v)\n", err.AutoStack(nil, nil)))
	} else {
		for _, process := range nodeList {
			logFile.WriteAccessLog(fmt.Sprintf("Daemon process get node, node=(%v)\n", process))
		}
	}

	defer daemon.Close()

	<-daemon.ExitCh
}

func main() {
	var AtomosRunnable atomos.CosmosRunnable
	AtomosRunnable.SetScript(func(main *atomos.CosmosMain, killSignal chan bool) {})
	Main(AtomosRunnable)
}
