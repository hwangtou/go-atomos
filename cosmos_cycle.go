package go_atomos

type DaemonCommandType int

const (
	DaemonCommandExecuteRunnable = 1
	DaemonCommandExecuteDynamicRunnable = 2
)

type DaemonCommand struct {
	Cmd DaemonCommandType
	Runnable CosmosRunnable
	DynamicRunnable CosmosDynamicRunnable
}
