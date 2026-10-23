package atomos

import (
	"flag"
	"fmt"
	"log"
	"os"
)

// RunOption configures the behavior of Run. Options are applied in order;
// when several options overlap, the last one wins.
type RunOption func(*runConfig)

type runConfig struct {
	configFile    string
	hasConfigFile bool
	standalone    bool

	useWorkingPath bool
	workingPath    string
	cosmos         string
	node           string
	logLevel       LogLevel
	customize      map[string][]byte
}

// WithConfigFile makes Run load the YAML config from path instead of the
// --config command-line flag. An explicit option always wins over the flag.
func WithConfigFile(path string) RunOption {
	return func(c *runConfig) {
		c.configFile = path
		c.hasConfigFile = true
	}
}

// WithWorkingPath makes Run start with an explicit working directory, Cosmos
// name, and node name rather than reading a config file. This is useful for
// embedded scenarios and programmatically built configuration. A process
// started this way never forks a daemon child; it always runs directly.
func WithWorkingPath(path, cosmos, node string, logLevel LogLevel, customize map[string][]byte) RunOption {
	return func(c *runConfig) {
		c.useWorkingPath = true
		c.workingPath = path
		c.cosmos = cosmos
		c.node = node
		c.logLevel = logLevel
		c.customize = customize
	}
}

// WithStandalone forces standalone mode: the process never forks a daemon
// child. It is equivalent to the --standalone flag, the
// ATOMOS_STANDALONE=true environment variable, or running inside a Docker
// container (which is auto-detected).
func WithStandalone() RunOption {
	return func(c *runConfig) {
		c.standalone = true
	}
}

// Run is the single process entry point of Atomos. By default it reads the
// --config and --standalone command-line flags, loads the YAML config file,
// and starts the Cosmos process. In production the parent process forks a
// child daemon and exits; the child runs the actor runtime. Use
// WithStandalone (or --standalone, ATOMOS_STANDALONE=true, or Docker) to skip
// forking. Use atomostest.Run for unit tests.
func Run(runnable *CosmosRunnable, opts ...RunOption) {
	rc := &runConfig{}
	for _, opt := range opts {
		opt(rc)
	}

	log.Printf("Welcome to Atomos! pid=(%d)", os.Getpid())

	var (
		configPathFlag = flag.String("config", "", "config path")
		standaloneFlag = flag.Bool("standalone", false, "standalone")
	)
	flag.Parse()

	standalone := rc.standalone || boolFlag(standaloneFlag)
	// Auto-detect Docker: force standalone mode (skip forking).
	// ATOMOS_STANDALONE env var provides explicit override without flags.
	envStandalone := isRunningInDocker() || os.Getenv("ATOMOS_STANDALONE") == "true"
	if envStandalone {
		log.Printf("App: Running in container/standalone mode, fork skipped.")
	}

	var app *App
	var err error
	if rc.useWorkingPath {
		if rc.workingPath == "" {
			if wd, er := os.Getwd(); er == nil {
				log.Printf("App: Current path. path=(%s)", wd)
			}
		}
		app, err = NewCosmosNodeAppWithWorkingPath(*runnable, rc.workingPath, rc.cosmos, rc.node, rc.logLevel, rc.customize)
	} else {
		configPath := *configPathFlag
		if rc.hasConfigFile {
			configPath = rc.configFile
		}
		if configPath == "" {
			log.Println("App: No config path specified.", os.Getpid())
			os.Exit(1)
		}
		app, err = NewCosmosNodeAppWithConfigPath(configPath, runnable)
	}
	if err != nil {
		log.Printf("App: Config is invalid. pid=(%d),err=(%v)", os.Getpid(), err)
		os.Exit(1)
	}

	process, err := app.NewCosmosProcess()
	if err != nil {
		log.Printf("App: Init cosmos process failed. pid=(%d),err=(%v)", os.Getpid(), err)
		os.Exit(1)
	}

	// In Docker, skip PID-based duplicate-instance detection.
	// Container runtime guarantees single-instance-per-container.
	if !isRunningInDocker() {
		checkAppOrExit(app, process)
	}

	// Working-path mode never forks; config mode forks a daemon child unless
	// standalone was requested via option, flag, env var, or Docker detection.
	if !rc.useWorkingPath && IsParentProcess() && !standalone && !envStandalone {
		if err = app.ForkAppProcess(); err != nil {
			logAndExit(process, "App: Fork app failed. err=(%v)", err)
		}
		logAndInfo(process, "App: Fork app succeed. Loader will exit.")
		return
	}
	if rc.useWorkingPath {
		// Working-path mode launches the app environment up front; launchAndRun
		// launches it again (daemon setup is idempotent over the PID file).
		if err = app.LaunchApp(); err != nil {
			logAndExit(process, "App: Launch app failed. err=(%v)", err)
		}
	}
	launchAndRun(app, process, runnable)
}

// Main is the legacy entry point, equivalent to Run with no options: it reads
// --config and --standalone flags, loads the YAML config file, and starts the
// Cosmos process, forking a daemon child in production.
//
// Deprecated: use Run instead.
func Main(runnable CosmosRunnable) {
	Run(&runnable)
}

// MainForWorkingPath starts the framework with an explicit working directory,
// Cosmos name, and node name, rather than reading a config file. It never
// forks; it always runs directly.
//
// Deprecated: use Run with WithWorkingPath instead.
func MainForWorkingPath(runnable CosmosRunnable, path, cosmos, node string, logLevel LogLevel, customize map[string][]byte) {
	Run(&runnable, WithWorkingPath(path, cosmos, node, logLevel, customize))
}

// ━━━ Internal Helpers ━━━

func boolFlag(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func logAndExit(process *CosmosProcess, format string, err error) {
	msg := fmt.Sprintf(format, err)
	process.Self().coreLog().coreFatal(msg)
	log.Print(msg)
	os.Exit(1)
}

func logAndInfo(process *CosmosProcess, msg string) {
	process.Self().coreLog().coreInfo(msg)
	log.Print(msg)
}

// checkAppOrExit aborts the process if another instance is already running
// (PID-file based duplicate-instance detection).
func checkAppOrExit(app *App, process *CosmosProcess) {
	isRunning, processID, err := app.Check()
	if err != nil && !isRunning {
		msg := fmt.Sprintf("App: Check failed. err=(%v)", err)
		process.Self().coreLog().coreFatal(msg)
		log.Print(msg)
		os.Exit(1)
	}
	if isRunning {
		msg := fmt.Sprintf("App: App is already running. pid=(%d)", processID)
		process.Self().coreLog().coreFatal(msg)
		log.Print(msg)
		os.Exit(1)
	}
}

func launchAndRun(app *App, process *CosmosProcess, runnable *CosmosRunnable) {
	if err := app.LaunchApp(); err != nil {
		logAndExit(process, "App: Launch app failed. err=(%v)", err)
	}

	defer func() {
		process.Self().coreLog().coreInfo("App: Exiting.")
		app.close()
	}()
	runnable.SetConfig(app.config)
	if err := process.Start(runnable); err != nil {
		process.Self().coreLog().coreFatal("App: Runnable starts failed, now exiting. err=(%v)", asError(err).AddStack(nil))
		return
	}
	process.Self().coreLog().coreInfo("App: Started.")
	<-app.WaitExitApp()
	if err := process.Stop(); err != nil {
		process.Self().coreLog().coreFatal("App: Runnable stops with error. err=(%v)", asError(err).AddStack(nil))
	}
}
