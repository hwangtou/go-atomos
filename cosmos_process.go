package go_atomos

// Cosmos生命周期，开发者定义和使用的部分。
// Cosmos Life Cycle, part for developer customize and use.

// CosmosProcess
// 这个才是进程的主循环。

type CosmosProcess struct {
	sharedLog *LoggingAtomos

	// CosmosRunnable & CosmosMainFn.
	// 可运行Cosmos & Cosmos运行时。

	// Loads at DaemonWithRunnable or Runnable.
	main *CosmosMain
}
