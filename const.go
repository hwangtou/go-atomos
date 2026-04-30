package atomos

// loggingAtomos
const (
	logTimeFmt = "2006-01-02 15:04:05.000000"
	logTestOut = true
	logTestErr = true
)

//var (
//	LogStdout = false
//	LogStderr = false
//)
//
//func SetLogStdout(b bool) {
//	LogStdout = b
//}
//
//func SetLogStderr(b bool) {
//	LogStderr = b
//}

const (
	udsConnReadBufSize = 1024
)

const (
	ShouldArgumentClone = false
)

const (
	ElementBroadcastName = "Broadcast"
)

var muteKeepaliveLog = true

func MuteKeepaliveLog(b bool) {
	muteKeepaliveLog = b
}

const (
	GRPCServerInitialWindowSize     = 1024 * 128      // 128K	默认 64K
	GRPCServerInitialConnWindowSize = 1024 * 1024 * 1 // 1M	默认 64K
	GRPCServerWriteBufferSize       = 1024 * 32       // 32K	默认 32KB
	GRPCServerReadBufferSize        = 1024 * 32       // 32K	默认 32KB

	GRPCClientInitialWindowSize     = 1024 * 128      // 128K	默认 64K
	GRPCClientInitialConnWindowSize = 1024 * 1024 * 1 // 1M	默认 64K
	GRPCClientWriteBufferSize       = 1024 * 32       // 32K	默认 32KB
	GRPCClientReadBufferSize        = 1024 * 32       // 32K	默认 32KB
)
