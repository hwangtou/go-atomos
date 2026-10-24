package atomos

// loggingAtomos
const (
	logTimeFmt = "2006-01-02 15:04:05.000000"
	logTestOut = true
	logTestErr = true
)

const (
	udsConnReadBufSize  = 1024
	shouldArgumentClone = false

	ElementBroadcastName = "Broadcast"

	// configKeyLogStdout is a key in Config.Customize that, when set to "1",
	// forces all logging to stdout/stderr instead of files. Set via
	// ATOMOS_LOG_STDOUT env var, YAML log-std field, or Docker auto-detection.
	configKeyLogStdout = "_log_stdout"

	// configKeyDebugServer is a key in Config.Customize whose value is the
	// TCP listen address of the optional debug server (e.g. "127.0.0.1:9099").
	// When present, CosmosProcess.Start serves /elements, /metrics, /healthz
	// and /debug/pprof/* on that address until Stop. Set via the YAML
	// customize map or the WithDebugServer Run option.
	configKeyDebugServer = "_debug_server"

	grpcServerInitialWindowSize     = 1024 * 128      // 128K
	grpcServerInitialConnWindowSize = 1024 * 1024 * 1 // 1M
	grpcServerWriteBufferSize       = 1024 * 32       // 32K
	grpcServerReadBufferSize        = 1024 * 32       // 32K

	grpcClientInitialWindowSize     = 1024 * 128      // 128K
	grpcClientInitialConnWindowSize = 1024 * 1024 * 1 // 1M
	grpcClientWriteBufferSize       = 1024 * 32       // 32K
	grpcClientReadBufferSize        = 1024 * 32       // 32K
)
