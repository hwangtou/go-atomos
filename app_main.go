package go_atomos

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
)

var app *App
var nodeOrSupervisor = true
var supervisorCommandHandlers map[string]AppUDSCommandFn

func SharedApp() *App {
	return app
}

func SupervisorSet(handlers map[string]AppUDSCommandFn) {
	nodeOrSupervisor = false
	supervisorCommandHandlers = handlers
}

func Main(runnable CosmosRunnable) {
	if nodeOrSupervisor {
		log.Printf("Welcome to Atomos! pid=(%d)", os.Getpid())
	} else {
		log.Printf("Welcome to Atomos Supervisor! pid=(%d)", os.Getpid())
	}

	var (
		configPath = flag.String("config", "", "config path")
		standalone = flag.Bool("standalone", false, "standalone")
	)
	flag.Parse()

	var err *Error

	// Load Config.
	if configPath == nil {
		log.Println("App: No config path specified.", os.Getpid())
		os.Exit(1)
	}

	// Check.
	if nodeOrSupervisor {
		app, err = NewCosmosNodeApp(*configPath)
	} else {
		app, err = NewCosmosSupervisorApp(*configPath)
	}
	if err != nil {
		log.Printf("App: Config is invalid. pid=(%d),err=(%v)", os.Getpid(), err)
		os.Exit(1)
	}

	// Init.
	if err := InitCosmosProcess(app.config.Cosmos, app.config.Node, app.logging.WriteAccessLog, app.logging.WriteErrorLog); err != nil {
		log.Printf("App: Init cosmos process failed. pid=(%d),err=(%v)", os.Getpid(), err)
		os.Exit(1)
	}

	isRunning, processID, err := app.Check()
	if err != nil && !isRunning {
		msg := fmt.Sprintf("App: Check failed. err=(%v)", err)
		SharedCosmosProcess().Self().Log().Fatal(msg)
		log.Printf(msg)
		os.Exit(1)
	}
	if isRunning {
		msg := fmt.Sprintf("App: App is already running. pid=(%d)", processID)
		SharedCosmosProcess().Self().Log().Fatal(msg)
		log.Printf(msg)
		os.Exit(1)
	}

	sa := false
	if standalone != nil {
		sa = *standalone
	}
	if IsParentProcess() && !sa {
		if err = app.ForkAppProcess(); err != nil {
			msg := fmt.Sprintf("App: Fork app failed. err=(%v)", err)
			SharedCosmosProcess().Self().Log().Fatal(msg)
			log.Printf(msg)
			os.Exit(1)
		}
		msg := fmt.Sprintf("App: Fork app succeed. Loader will exit.")
		SharedCosmosProcess().Self().Log().Fatal(msg)
		log.Printf(msg)
		log.Printf("App: Access Log File=(%s)", app.logging.curAccessLogName)
		log.Printf("App: Error Log File=(%s)", app.logging.curErrorLogName)
		app.logging.Close()
		return
	} else {
		dFn, err := redirectSTD()
		if err != nil {
			msg := fmt.Sprintf("App: Redirect STD failed. err=(%v)", err)
			SharedCosmosProcess().Self().Log().Fatal(msg)
			log.Printf(msg)
			os.Exit(1)
		}
		if dFn != nil {
			defer dFn()
		}

		if err = app.LaunchApp(); err != nil {
			msg := fmt.Sprintf("App: Launch app failed. err=(%v)", err)
			SharedCosmosProcess().Self().Log().Fatal(msg)
			log.Printf(msg)
			os.Exit(1)
		}

		defer func() {
			SharedCosmosProcess().Self().Log().Fatal("App: Exiting.")
			app.close()
		}()
		runnable.SetConfig(app.config)
		if err = SharedCosmosProcess().Start(&runnable); err != nil {
			SharedCosmosProcess().Self().Log().Error("App: Runnable starts failed. err=(%v)", err.AddStack(nil))
			return
		}
		SharedCosmosProcess().Self().Log().Fatal("App: Started.")
		<-app.WaitExitApp()
		if err = SharedCosmosProcess().Stop(); err != nil {
			SharedCosmosProcess().Self().Log().Error("App: Runnable stops with error. err=(%v)", err.AddStack(nil))
		}
		return
	}
}

// InitCosmosProcess 初始化进程
// 该函数只能被调用一次，且必须在进程启动时调用。
func InitCosmosProcess(cosmosName, cosmosNode string, accessLogFn, errLogFn loggingFn) (err *Error) {
	onceInitSharedCosmosProcess.Do(func() {
		sharedCosmosProcess, err = newCosmosProcess(cosmosName, cosmosNode, accessLogFn, errLogFn)
	})
	return
}

var sharedCosmosProcess *CosmosProcess
var onceInitSharedCosmosProcess sync.Once

func SharedCosmosProcess() *CosmosProcess {
	return sharedCosmosProcess
}

func redirectSTD() (func(), *Error) {
	reader, writer, er := os.Pipe()
	if er != nil {
		return nil, NewErrorf(ErrFrameworkInternalError, "App: RedirectSTD create pipe failed. err=(%v)", er)
	}

	os.Stdout = writer
	os.Stderr = writer

	out := make(chan string)
	go func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			out <- scanner.Text()
		}
	}()

	go func() {
		for str := range out {
			SharedCosmosProcess().logging.errorLog(str)
		}
	}()

	// Ensure that the writes finish before we exit.
	return func() {
		writer.Close()
		reader.Close()
		close(out)
	}, nil
}
