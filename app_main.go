package go_atomos

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
)

var app *App

func MainForConfigFile(runnable CosmosRunnable) {
	log.Printf("Welcome to Atomos! pid=(%d)", os.Getpid())

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
	app, err = NewCosmosNodeAppWithConfigPath(*configPath, &runnable)
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
		SharedCosmosProcess().Self().Log().coreFatal(msg)
		log.Printf(msg)
		os.Exit(1)
	}
	if isRunning {
		msg := fmt.Sprintf("App: App is already running. pid=(%d)", processID)
		SharedCosmosProcess().Self().Log().coreFatal(msg)
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
			SharedCosmosProcess().Self().Log().coreFatal(msg)
			log.Printf(msg)
			os.Exit(1)
		}
		msg := fmt.Sprintf("App: Fork app succeed. Loader will exit.")
		SharedCosmosProcess().Self().Log().coreInfo(msg)
		log.Printf(msg)
		log.Printf("App: Access Log File=(%s)", app.logging.getCurAccessLogName())
		log.Printf("App: Error Log File=(%s)", app.logging.getCurErrorLogName())
		app.logging.Close()
		return
	} else {
		if err = app.LaunchApp(); err != nil {
			msg := fmt.Sprintf("App: Launch app failed. err=(%v)", err)
			SharedCosmosProcess().Self().Log().coreFatal(msg)
			log.Printf(msg)
			os.Exit(1)
		}

		defer func() {
			SharedCosmosProcess().Self().Log().coreInfo("App: Exiting.")
			app.close()
		}()
		runnable.SetConfig(app.config)
		if err = SharedCosmosProcess().Start(&runnable); err != nil {
			SharedCosmosProcess().Self().Log().coreFatal("App: Runnable starts failed, now exiting. err=(%v)", err.AddStack(nil))
			return
		}
		SharedCosmosProcess().Self().Log().coreInfo("App: Started.")
		<-app.WaitExitApp()
		if err = SharedCosmosProcess().Stop(); err != nil {
			SharedCosmosProcess().Self().Log().coreFatal("App: Runnable stops with error. err=(%v)", err.AddStack(nil))
		}
		return
	}
}

func MainForWorkingPath(runnable CosmosRunnable, path, cosmos, node string, logLevel LogLevel, customize map[string][]byte) {
	log.Printf("Welcome to Atomos! pid=(%d)", os.Getpid())

	var (
		standalone = flag.Bool("standalone", false, "standalone")
	)
	flag.Parse()

	// Get current path
	if path == "" {
		if wd, er := os.Getwd(); er == nil {
			log.Printf("App: Current path. path=(%s)", wd)
		}
	}

	app, err := NewCosmosNodeAppWithWorkingPath(runnable, path, cosmos, node, logLevel, customize)
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
		SharedCosmosProcess().Self().Log().coreFatal(msg)
		log.Printf(msg)
		os.Exit(1)
	}
	if isRunning {
		msg := fmt.Sprintf("App: App is already running. pid=(%d)", processID)
		SharedCosmosProcess().Self().Log().coreFatal(msg)
		log.Printf(msg)
		os.Exit(1)
	}

	sa := false
	if standalone != nil {
		sa = *standalone
	}
	if IsParentProcess() && !sa {
		if err = app.LaunchApp(); err != nil {
			msg := fmt.Sprintf("App: Launch app failed. err=(%v)", err)
			SharedCosmosProcess().Self().Log().coreFatal(msg)
			log.Printf(msg)
			os.Exit(1)
		}

		defer func() {
			SharedCosmosProcess().Self().Log().coreInfo("App: Exiting.")
			app.close()
		}()
		runnable.SetConfig(app.config)
		if err = SharedCosmosProcess().Start(&runnable); err != nil {
			SharedCosmosProcess().Self().Log().coreFatal("App: Runnable starts failed, now exiting. err=(%v)", err.AddStack(nil))
			return
		}
		SharedCosmosProcess().Self().Log().coreInfo("App: Started.")
		<-app.WaitExitApp()
		if err = SharedCosmosProcess().Stop(); err != nil {
			SharedCosmosProcess().Self().Log().coreFatal("App: Runnable stops with error. err=(%v)", err.AddStack(nil))
		}
		return
	}
}

func MainForTest(runnable CosmosRunnable, t *testing.T) {
	t.Logf("Welcome to Atomos! pid=(%d)", os.Getpid())

	var err *Error

	// Check.
	app, err = NewCosmosNodeAppWithTest(runnable.config, t)
	if err != nil {
		log.Printf("App: Config is invalid. pid=(%d),err=(%v)", os.Getpid(), err)
		os.Exit(1)
	}

	// Init.
	if err := InitCosmosProcess(app.config.Cosmos, app.config.Node, app.logging.WriteAccessLog, app.logging.WriteErrorLog); err != nil {
		log.Printf("App: Init cosmos process failed. pid=(%d),err=(%v)", os.Getpid(), err)
		os.Exit(1)
	}

	runnable.SetConfig(app.config)
	if err = SharedCosmosProcess().Start(&runnable); err != nil {
		SharedCosmosProcess().Self().Log().coreFatal("App: Runnable starts failed, now exiting. err=(%v)", err.AddStack(nil))
		return
	}
	SharedCosmosProcess().Self().Log().coreInfo("App: Started.")
	return
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
