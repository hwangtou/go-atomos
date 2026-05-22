package atomos

// loggingAtomos
const (
	logTimeFmt = "2006-01-02 15:04:05.000000"
	logTestOut = true
	logTestErr = true
)

const (
	udsConnReadBufSize  = 1024
	ShouldArgumentClone = false

	ElementBroadcastName = "Broadcast"

	// ConfigKeyLogStdout is a key in Config.Customize that, when set to "1",
	// forces all logging to stdout/stderr instead of files. Set via
	// ATOMOS_LOG_STDOUT env var, YAML log-std field, or Docker auto-detection.
	ConfigKeyLogStdout = "_log_stdout"

	GRPCServerInitialWindowSize     = 1024 * 128      // 128K
	GRPCServerInitialConnWindowSize = 1024 * 1024 * 1 // 1M
	GRPCServerWriteBufferSize       = 1024 * 32       // 32K
	GRPCServerReadBufferSize        = 1024 * 32       // 32K

	GRPCClientInitialWindowSize     = 1024 * 128      // 128K
	GRPCClientInitialConnWindowSize = 1024 * 1024 * 1 // 1M
	GRPCClientWriteBufferSize       = 1024 * 32       // 32K
	GRPCClientReadBufferSize        = 1024 * 32       // 32K
)
