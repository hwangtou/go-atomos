package main

import (
	atomos "github.com/hwangtou/go-atomos"
	"hello_atomos/api"
	"hello_atomos/hello"
	"hello_atomos/hello/views"
)

func main() {
	atomos.Main(AtomosRunnable)
}

var AtomosRunnable atomos.CosmosRunnable

var (
	TestMode  = true
	TestCases = []api.HADoTestI_TestMode{
		api.HADoTestI_SyncSelfCallDeadlock,
		api.HADoTestI_AsyncSelfCallNoDeadlock,
		api.HADoTestI_SyncRingCallDeadlock,
		api.HADoTestI_AsyncRingCallNoDeadlock,
		api.HADoTestI_AsyncRingCallDeadlock,
		api.HADoTestI_AsyncRingCallNoReplyNoDeadlock,
		api.HADoTestI_AsyncRingCallNoReplyDeadlock,
	}
)

func init() {
	AtomosRunnable.
		SetConfig(&atomos.Config{}).
		AddElementImplementation(api.GetHelloAtomosImplement(hello.NewDev())).
		SetMainScript(&hellMain{})
}

type hellMain struct {
	local *atomos.CosmosProcess
}

func (m *hellMain) OnBoot(local *atomos.CosmosProcess) *atomos.Error {
	m.local = local
	return nil
}

func (m *hellMain) OnStartUp(local *atomos.CosmosProcess) *atomos.Error {
	if TestMode {
		return m.testing(local)
	}
	return nil
}

func (m *hellMain) testing(local *atomos.CosmosProcess) *atomos.Error {
	hello1, err := api.SpawnHelloAtomosAtom(local.Self(), local.Self().CosmosMain(), "hello1", &api.HASpawnArg{Id: 1})
	if err != nil {
		return err.AddStack(local.Self())
	}
	defer hello1.Release()
	local.Self().Log().Info("Spawned: %v", hello1)

	hello2, err := api.SpawnHelloAtomosAtom(local.Self(), local.Self().CosmosMain(), "hello2", &api.HASpawnArg{Id: 1})
	if err != nil {
		return err.AddStack(local.Self())
	}
	defer hello2.Release()
	local.Self().Log().Info("Spawned: %v", hello2)

	for _, mode := range TestCases {
		views.TestMapMutex.Lock()
		views.TestMap[mode] = 1
		views.TestMapMutex.Unlock()
		if _, err := hello1.DoTest(local.Self(), &api.HADoTestI{Mode: mode}); err != nil {
			return err.AddStack(local.Self())
		}
		local.Self().Log().Info("DoTest: [%v] PASS", mode)
	}
	return nil
}

func (m *hellMain) OnShutdown() *atomos.Error {
	if TestMode {
		m.testResult()
	}
	return nil
}

func (m *hellMain) testResult() {
	views.TestMapMutex.Lock()
	pass := true
	for _, mode := range TestCases {
		if views.TestMap[mode] != 2 {
			pass = false
		}
	}
	m.local.Self().Log().Info("TestResult: %v", pass)
	for _, mode := range TestCases {
		result := views.TestMap[mode]
		message := "PASS"
		if result != 2 {
			message = "FAIL"
		}
		m.local.Self().Log().Info("%s [%v]", message, mode)
	}
	views.TestMapMutex.Unlock()
}
