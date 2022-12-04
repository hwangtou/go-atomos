package main

import (
	"flag"
	"fmt"
	atomos "github.com/hwangtou/go-atomos"
	"log"
	"os"
	"strings"
	"time"
)

func Main(runnable atomos.CosmosRunnable) *atomos.Error {
	log.Println("Welcome to Atomos!", os.Getpid())
	defer log.Println("Bye!")

	var (
		configPath = flag.String("config", "", "config path")
	)

	flag.Parse()

	var err *atomos.Error
	conf, err := loadConfig(*configPath)
	if err != nil {
		return err.AutoStack(nil, nil)
	}

	// Open Log.
	logFile, err := atomos.NewLogFile(conf.LogPath)
	if err != nil {
		return err.AutoStack(nil, nil)
	}
	defer logFile.Close()

	cmd := &command{configPath: *configPath}
	daemon := atomos.NewCosmosDaemon(conf, logFile, cmd)
	hasRun, err := daemon.Check()
	if err != nil && !hasRun {
		logFile.WriteAccessLog(fmt.Sprintf("Daemon process check failed, err=(%v)\n", err))
		return err.AutoStack(nil, nil)
	}
	if hasRun {
		logFile.WriteAccessLog(fmt.Sprintf("Daemon process has run, err=(%v)\n", err))
		return nil
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

func loadConfig(configPath string) (*atomos.Config, *atomos.Error) {
	// Load Config.
	conf, err := atomos.NodeConfigFromYaml(configPath)
	if err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	if err = conf.Check(); err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	return conf, nil
}

func main() {
	var AtomosRunnable atomos.CosmosRunnable
	AtomosRunnable.SetScript(func(main *atomos.CosmosMain, killSignal chan bool) {})
	Main(AtomosRunnable)
}

type command struct {
	configPath string
	daemon     atomos.CosmosDaemon
}

func (c *command) Command(cmd string) (string, bool, *atomos.Error) {
	if len(cmd) > 0 {
		i := strings.Index(cmd, " ")
		if i == -1 {
			i = len(cmd)
		}
		switch cmd[:i] {
		case "status":
			return c.handleCommandStatus()
		case "start all":
			return c.handleCommandStartAll()
		case "stop all":
			return c.handleCommandStopAll()
		case "exit":
			return "closing", true, nil
		}
	}
	return fmt.Sprintf("invalid command \"%s\"\n", cmd), false, nil
}

func (c *command) Greeting() string {
	return "Hello Atomos!\n"
}

func (c *command) PrintNow() string {
	return "Now:\t" + time.Now().Format("2006-01-02 15:04:05 MST -07:00")
}

func (c *command) Bye() string {
	return "Bye Atomos!\n"
}
