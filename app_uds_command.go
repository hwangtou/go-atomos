package go_atomos

type AppUDSCommandFn func([]byte) ([]byte, *Error)
type AppUDSClientCallback func(*UDSCommandPacket, *Error)

const (
	UDSInvalidCommand = "InvalidCommand"
	UDSInvalidPacket  = "InvalidPacket"

	UDSPing    = "ping"
	UDSStatus  = "status"
	UDSStart   = "start"
	UDSStop    = "stop"
	UDSRestart = "restart"
)

var udsNodeCommandHandler = map[string]AppUDSCommandFn{
	UDSPing: udsPing,
}

func udsPing(bytes []byte) ([]byte, *Error) {
	return []byte("pong"), nil
}

func udsShutdown(bytes []byte) ([]byte, *Error) {
	p := SharedCosmosProcess()
	if p == nil {
		return nil, NewError(ErrCosmosIsClosed, "node is not running").AddStack(nil)
	}
	if err := p.Stop(); err != nil {
		return nil, err.AddStack(nil)
	}
	return []byte("ok"), nil
}
