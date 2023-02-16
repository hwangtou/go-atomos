package main

import atomos "github.com/hwangtou/go-atomos"

func main() {
	var runnable atomos.CosmosRunnable
	runnable.SetMainScript(&mainScript{})

	atomos.Main(runnable)
}

type mainScript struct {
}

func (m *mainScript) OnStartup() *atomos.Error {
	return nil
}

func (m *mainScript) OnShutdown() *atomos.Error {
	return nil
}
