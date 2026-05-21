package atomos

import (
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
)

var app *App

// Main is the primary entry point. It reads --config and --standalone flags,
// loads the YAML config file, and starts the Cosmos process. In production,
// the parent process forks a child daemon and exits; the child runs the
// actor runtime. Use --standalone to skip forking (e.g. under systemd).
func Main(runnable CosmosRunnable) {
	log.Printf("Welcome to Atomos! pid=(%d)", os.Getpid())

	var (
		configPath = flag.String("config", "", "config path")
		standalone = flag.Bool("standalone", false, "standalone")
	)
	flag.Parse()

	var err *Error
	if configPath == nil {
		log.Println("App: No config path specified.", os.Getpid())
		os.Exit(1)
	}

	app, err = NewCosmosNodeAppWithConfigPath(*configPath, &runnable)
	if err != nil {
		log.Printf("App: Config is invalid. pid=(%d),err=(%v)", os.Getpid(), err)
		os.Exit(1)
	}

	initAndCheckApp()

	if IsParentProcess() && !boolFlag(standalone) {
		if err = app.ForkAppProcess(); err != nil {
			logAndExit("App: Fork app failed. err=(%v)", err)
		}
		logAndInfo("App: Fork app succeed. Loader will exit.")
		return
	}
	launchAndRun(&runnable)
}

// MainForConfigFile is an alias for Main. Prefer Main for new code.
func MainForConfigFile(runnable CosmosRunnable) {
	Main(runnable)
}

// MainForWorkingPath starts the framework with an explicit working directory,
// Cosmos name, and node name, rather than reading a config file. This is
// useful for embedded scenarios, tests, and when configuration is built
// programmatically.
func MainForWorkingPath(runnable CosmosRunnable, path, cosmos, node string, logLevel LogLevel, customize map[string][]byte) {
	log.Printf("Welcome to Atomos! pid=(%d)", os.Getpid())

	flag.Parse()

	if path == "" {
		if wd, er := os.Getwd(); er == nil {
			log.Printf("App: Current path. path=(%s)", wd)
		}
	}

	var err *Error
	app, err = NewCosmosNodeAppWithWorkingPath(runnable, path, cosmos, node, logLevel, customize)
	if err != nil {
		log.Printf("App: Config is invalid. pid=(%d),err=(%v)", os.Getpid(), err)
		os.Exit(1)
	}

	initAndCheckApp()

	// MainForWorkingPath does not fork; it always runs directly.
	if err = app.LaunchApp(); err != nil {
		logAndExit("App: Launch app failed. err=(%v)", err)
	}
	launchAndRun(&runnable)
}

// MainForTest starts the framework in test mode. Logs go to t.Log
// instead of files, and the process check + fork steps are skipped.
func MainForTest(runnable CosmosRunnable, t *testing.T) {
	t.Logf("Welcome to Atomos! pid=(%d)", os.Getpid())

	var err *Error
	app, err = NewCosmosNodeAppWithTest(runnable.config, t)
	if err != nil {
		log.Printf("App: Config is invalid. pid=(%d),err=(%v)", os.Getpid(), err)
		os.Exit(1)
	}

	if err := InitCosmosProcess(app.config.Cosmos, app.config.Node, app.logging); err != nil {
		log.Printf("App: Init cosmos process failed. pid=(%d),err=(%v)", os.Getpid(), err)
		os.Exit(1)
	}

	runnable.SetConfig(app.config)
	if err = SharedCosmosProcess().Start(&runnable); err != nil {
		SharedCosmosProcess().Self().Log().coreFatal("App: Runnable starts failed, now exiting. err=(%v)", err.AddStack(nil))
		return
	}
	SharedCosmosProcess().Self().Log().coreInfo("App: Started.")
}

// ━━━ Internal Helpers ━━━

func boolFlag(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func logAndExit(format string, err *Error) {
	msg := fmt.Sprintf(format, err)
	SharedCosmosProcess().Self().Log().coreFatal(msg)
	log.Print(msg)
	os.Exit(1)
}

func logAndInfo(msg string) {
	SharedCosmosProcess().Self().Log().coreInfo(msg)
	log.Print(msg)
}

func initAndCheckApp() {
	if err := InitCosmosProcess(app.config.Cosmos, app.config.Node, app.logging); err != nil {
		log.Printf("App: Init cosmos process failed. pid=(%d),err=(%v)", os.Getpid(), err)
		os.Exit(1)
	}

	isRunning, processID, err := app.Check()
	if err != nil && !isRunning {
		msg := fmt.Sprintf("App: Check failed. err=(%v)", err)
		SharedCosmosProcess().Self().Log().coreFatal(msg)
		log.Print(msg)
		os.Exit(1)
	}
	if isRunning {
		msg := fmt.Sprintf("App: App is already running. pid=(%d)", processID)
		SharedCosmosProcess().Self().Log().coreFatal(msg)
		log.Print(msg)
		os.Exit(1)
	}
}

func launchAndRun(runnable *CosmosRunnable) {
	if err := app.LaunchApp(); err != nil {
		msg := fmt.Sprintf("App: Launch app failed. err=(%v)", err)
		SharedCosmosProcess().Self().Log().coreFatal(msg)
		log.Print(msg)
		os.Exit(1)
	}

	defer func() {
		SharedCosmosProcess().Self().Log().coreInfo("App: Exiting.")
		app.close()
	}()
	runnable.SetConfig(app.config)
	if err := SharedCosmosProcess().Start(runnable); err != nil {
		SharedCosmosProcess().Self().Log().coreFatal("App: Runnable starts failed, now exiting. err=(%v)", err.AddStack(nil))
		return
	}
	SharedCosmosProcess().Self().Log().coreInfo("App: Started.")
	<-app.WaitExitApp()
	if err := SharedCosmosProcess().Stop(); err != nil {
		SharedCosmosProcess().Self().Log().coreFatal("App: Runnable stops with error. err=(%v)", err.AddStack(nil))
	}
}

// ━━━ Process Management ━━━

// InitCosmosProcess initializes the singleton CosmosProcess. Must be called
// once before Start(). Safe to call multiple times (subsequent calls are no-ops).
func InitCosmosProcess(cosmosName, cosmosNode string, logging appLogging, args ...any) (err *Error) {
	onceInitSharedCosmosProcess.Do(func() {
		sharedCosmosProcess, err = newCosmosProcess(cosmosName, cosmosNode, logging, args...)
	})
	return
}

var sharedCosmosProcess *CosmosProcess
var onceInitSharedCosmosProcess sync.Once

// SharedCosmosProcess returns the singleton CosmosProcess for this application.
func SharedCosmosProcess() *CosmosProcess {
	return sharedCosmosProcess
}
