package go_atomos

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
