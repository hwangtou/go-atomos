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

var conf *atomos.Config
var nodeList []*nodeProcess

func main() {
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

func loadConfig(configPath string) (*atomos.Config, *atomos.Error) {
	// Load Config.
	conf, err := atomos.SupervisorConfigFromYaml(configPath)
	if err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	if err = conf.Check(); err != nil {
		return nil, err.AutoStack(nil, nil)
	}
	return conf, nil
}

type command struct {
	configPath string
	daemon     *atomos.CosmosDaemon
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

func (c *command) handleCommandStatus() (string, bool, *atomos.Error) {
	buf := strings.Builder{}
	for _, process := range nodeList {
		buf.WriteString(fmt.Sprintf("Node: %s, status=(%s)", process.config.Node, process.getStatus()))
	}
	return buf.String(), false, nil
}

func (c *command) handleCommandStartAll() (string, bool, *atomos.Error) {
	return "", false, nil
}

func (c *command) handleCommandStopAll() (string, bool, *atomos.Error) {
	return "stop all", false, nil
}

func (c *command) getNodeListConfig() ([]*nodeProcess, *atomos.Error) {
	//log.Printf("Daemon process load config failed, config=(%s),err=(%v)", *configPath, err)
	var nodeListConfig []*nodeProcess
	// Load Config.
	conf, err := atomos.SupervisorConfigFromYaml(c.configPath)
	if err != nil {
		return nodeListConfig, err.AutoStack(nil, nil)
	}
	if err = conf.Check(); err != nil {
		return nodeListConfig, err.AutoStack(nil, nil)
	}
	for _, node := range conf.NodeList {
		np := &nodeProcess{}
		nodeListConfig = append(nodeListConfig, np)

		np.confPath = conf.LogPath + "/" + node + ".conf"
		np.config, np.err = atomos.NodeConfigFromYaml(np.confPath)
		if err != nil {
			np.err = np.err.AutoStack(nil, nil)
			continue
		}
		if err = conf.Check(); err != nil {
			np.err = np.err.AutoStack(nil, nil)
			continue
		}
	}
	return nodeListConfig, nil
}

// Node Process

type nodeProcess struct {
	confPath string
	config   *atomos.Config
	err      *atomos.Error
}

func (p nodeProcess) getStatus() string {
	return ""
	//attr := &os.ProcAttr{
	//	Dir:   c.process.workPath,
	//	Env:   c.process.env,
	//	Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	//	Sys:   &syscall.SysProcAttr{Setsid: true},
	//}
	//proc, er = os.StartProcess(c.process.executablePath, c.process.args, attr)
	//if er != nil {
	//	return false, pid, NewError(ErrCosmosDaemonStartProcessFailed, er.Error()).AutoStack(nil, nil)
	//}
}
