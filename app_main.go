package atomos

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
)

// 若干種啓動模式：
// 1. standalone
// 2. daemon
// 3. embedded
// 4. test
// 5. benchmark

// 若干種配置方式，具體方法取決於啓動模式：
// 1. config file
// 2. code
// 3. etcd

var app *App

func MainForConfigFile(runnable CosmosRunnable, args ...any) {
	log.Printf("Welcome to Atomos! pid=(%d)", os.Getpid())

	var (
		configPath = flag.String("config", "", "config path")
		standalone = flag.Bool("standalone", false, "standalone")
	)
	flag.Parse()

	var critical bool
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
	if err := InitCosmosProcess(app.config.Cosmos, app.config.Node, app.logging, args...); err != nil {
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
		//log.Printf("App: Access Log File=(%s)") //, app.logging.getCurAccessLogName())
		//log.Printf("App: Error Log File=(%s)")  //, app.logging.getCurErrorLogName())
		//app.logging.Close()
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
		if critical, err = SharedCosmosProcess().start(&runnable); err != nil {
			if critical {
				SharedCosmosProcess().Self().Log().coreFatal("App: CosmosRunnable starts failed critically, now exiting. err=(%v)", err.AddStack(nil))
			} else {
				SharedCosmosProcess().Self().Log().coreFatal("App: CosmosRunnable starts failed, now exiting. err=(%v)", err.AddStack(nil))
			}
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

func MainForWorkingPath(runnable CosmosRunnable, path, cosmos, node string, logLevel LogLevel, customize map[string][]byte, args ...any) {
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

	var critical bool
	var err *Error

	app, err := NewCosmosNodeAppWithWorkingPath(runnable, path, cosmos, node, logLevel, customize)
	if err != nil {
		log.Printf("App: Config is invalid. pid=(%d),err=(%v)", os.Getpid(), err)
		os.Exit(1)
	}

	// Init.
	if err := InitCosmosProcess(app.config.Cosmos, app.config.Node, app.logging, args...); err != nil {
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
		if critical, err = SharedCosmosProcess().start(&runnable); err != nil {
			if critical {
				SharedCosmosProcess().Self().Log().coreFatal("App: Runnable starts failed critically, now exiting. err=(%v)", err.AddStack(nil))
			} else {
				SharedCosmosProcess().Self().Log().coreFatal("App: Runnable starts failed, now exiting. err=(%v)", err.AddStack(nil))
			}
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

func MainForTest(runnable CosmosRunnable, t *testing.T, args ...any) {
	t.Logf("Welcome to Atomos! pid=(%d)", os.Getpid())

	var critical bool
	var err *Error

	// Check.
	app, err = NewCosmosNodeAppWithTest(runnable.config, t)
	if err != nil {
		log.Printf("App: Config is invalid. pid=(%d),err=(%v)", os.Getpid(), err)
		t.Fatalf("App: Config is invalid. err=(%v)", err)
	}

	// Init.
	args = append(args, t)
	if err := InitCosmosProcess(app.config.Cosmos, app.config.Node, app.logging, args...); err != nil {
		log.Printf("App: Init cosmos process failed. pid=(%d),err=(%v)", os.Getpid(), err)
		t.Fatalf("App: Init cosmos process failed. err=(%v)", err)
	}

	runnable.SetConfig(app.config)
	if critical, err = SharedCosmosProcess().start(&runnable); err != nil {
		if critical {
			SharedCosmosProcess().Self().Log().coreFatal("App: Runnable starts failed critically, now exiting. err=(%v)", err.AddStack(nil))
		} else {
			SharedCosmosProcess().Self().Log().coreFatal("App: Runnable starts failed, now exiting. err=(%v)", err.AddStack(nil))
		}
		t.Fatalf("App: Runnable starts failed. err=(%v)", err.AddStack(nil))
	}
	SharedCosmosProcess().Self().Log().coreInfo("App: Started.")
	return
}

// InitCosmosProcess 初始化进程
// 该函数只能被调用一次，且必须在进程启动时调用。
func InitCosmosProcess(cosmosName, cosmosNode string, logging appLogging, args ...any) (err *Error) {
	onceInitSharedCosmosProcess.Do(func() {
		sharedCosmosProcess, err = newCosmosProcess(cosmosName, cosmosNode, logging, args...)
	})
	return
}

var sharedCosmosProcess *CosmosProcess
var onceInitSharedCosmosProcess sync.Once

func SharedCosmosProcess() *CosmosProcess {
	return sharedCosmosProcess
}
