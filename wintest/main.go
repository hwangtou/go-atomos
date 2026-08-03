//go:build windows

// wintest is a minimal Windows runtime verification program for go-atomos.
//
// 用法（在 Windows 上）：
//   wintest.exe              → 启动框架，Ctrl+C 退出
//   wintest.exe -standalone  → 显式 standalone 模式
//
// 验证项：
//   1. 进程启动（InitCosmosProcess + Start）
//   2. Element/Atom spawn 生命周期
//   3. 日志文件写入（Windows 文件路径/权限）
//   4. PID 文件创建/清理
//   5. Ctrl+C 信号处理 → 优雅退出
//
// Build (from macOS):
//   GOOS=windows GOARCH=amd64 go build -o /tmp/wintest.exe ./wintest
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	atomos "github.com/hwangtou/go-atomos"
	"google.golang.org/protobuf/proto"
)

// --- 最小 Element/Atom 实现 ---

type winDev struct{}

func (d *winDev) ElementConstructor() atomos.Atomos       { return &winElement{} }
func (d *winDev) AtomConstructor(name string) atomos.Atomos { return &winAtom{} }

type winElement struct {
	self atomos.ElementSelfID
}

func (e *winElement) String() string { return e.self.String() }

func (e *winElement) Spawn(self atomos.ElementSelfID, _ *atomos.Nil, _ ...atomos.ArgsForBaseAtomos) *atomos.Error {
	e.self = self
	self.Log().Info("[WINTEST] Element spawned")
	return nil
}

func (e *winElement) Halt(_ atomos.ID, _ []uint64) (bool, proto.Message) {
	return false, nil
}

type winAtom struct {
	self atomos.AtomSelfID
}

func (a *winAtom) String() string { return a.self.String() }

func (a *winAtom) Spawn(self atomos.AtomSelfID, _ *atomos.Nil, _ *atomos.Nil, _ ...atomos.ArgsForBaseAtomos) *atomos.Error {
	a.self = self
	self.Log().Info("[WINTEST] Atom spawned: %s", self.String())
	return nil
}

func (a *winAtom) Halt(_ atomos.ID, _ []uint64) (bool, proto.Message) {
	if a.self != nil {
		a.self.Log().Info("[WINTEST] Atom halting")
	}
	return false, nil
}

// --- MainScript ---

type winMain struct{}

func (m *winMain) OnBoot(_ *atomos.CosmosProcess) *atomos.Error {
	fmt.Println("[WINTEST] OnBoot OK")
	return nil
}

func (m *winMain) OnStartUp(p *atomos.CosmosProcess) *atomos.Error {
	fmt.Println("[WINTEST] OnStartUp OK")
	// 验证 element/atom spawn
	fmt.Println("[WINTEST] Spawning test atom...")
	atomID, _, err := p.Self().CosmosSpawnAtom(p.Self(), "WinTest", "test_atom_1", &atomos.Nil{})
	if err != nil {
		fmt.Printf("[WINTEST] SpawnAtom FAILED: %v\n", err)
		p.Self().Log().Error("[WINTEST] SpawnAtom FAILED: %v", err)
		return nil // 不 return err（会触发 os.Exit），记录错误让进程继续运行
	}
	fmt.Printf("[WINTEST] SpawnAtom OK: %s\n", atomID.GetIDInfo().Info())
	fmt.Printf("[WINTEST] IsHealthy: %v\n", p.IsHealthy())
	return nil
}

func (m *winMain) OnShutdown() *atomos.Error {
	fmt.Println("[WINTEST] OnShutdown OK")
	return nil
}

// --- 入口 ---

func main() {
	exePath, _ := os.Executable()
	workDir := filepath.Join(filepath.Dir(exePath), "wintest_run")
	os.MkdirAll(workDir, 0755)

	fmt.Println("============================================")
	fmt.Println("[WINTEST] go-atomos Windows Runtime Verify")
	fmt.Println("============================================")
	fmt.Printf("[WINTEST] exe:    %s\n", exePath)
	fmt.Printf("[WINTEST] workDir: %s\n", workDir)
	fmt.Printf("[WINTEST] pid:    %d\n", os.Getpid())
	fmt.Println("[WINTEST] Starting (standalone, no fork)...")
	fmt.Println("[WINTEST] Press Ctrl+C to test graceful shutdown.")
	fmt.Println()

	// 构造 runnable
	dev := &winDev{}
	impl := atomos.NewImplementationFromDeveloper(dev)
	impl.Interface = atomos.NewInterfaceFromDeveloper("WinTest", dev)
	// NewInterfaceFromDeveloper 不填 Spawner（正常由 protoc 生成代码填充），
	// 这里手动设置，否则 spawn 时会 panic（nil func call）。
	impl.Interface.ElementSpawner = func(self atomos.ElementSelfID, a atomos.Atomos, data proto.Message, _ ...atomos.ArgsForBaseAtomos) *atomos.Error {
		return a.(*winElement).Spawn(self, &atomos.Nil{})
	}
	impl.Interface.AtomSpawner = func(self atomos.AtomSelfID, a atomos.Atomos, arg, data proto.Message, _ ...atomos.ArgsForBaseAtomos) *atomos.Error {
		// arg/data 可能是 nil 或 *atomos.Nil，不强制断言
		return a.(*winAtom).Spawn(self, &atomos.Nil{}, &atomos.Nil{})
	}

	runnable := &atomos.CosmosRunnable{}
	runnable.
		SetMainScript(&winMain{}).
		AddElementImplementation(impl, true)

	// standalone 模式（Windows 不走 fork daemon）
	os.Setenv("ATOMOS_STANDALONE", "true")
	atomos.MainForWorkingPath(*runnable, workDir, "WinTestCosmos", "WinTestNode", atomos.LogLevel_Info, nil)

	fmt.Println("[WINTEST] Framework exited.")
	time.Sleep(300 * time.Millisecond)
}
