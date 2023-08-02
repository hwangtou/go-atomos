package go_atomos

import (
	"fmt"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/process"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"time"
)

type AppRuntimeInfo struct {
	cpuInfo []cpu.InfoStat
	process *process.Process

	profileMutex sync.Mutex
	cpuProfile   *os.File
	traceProfile *os.File
}

func NewAppRuntimeInfo() (*AppRuntimeInfo, *Error) {
	// Get detailed CPU info
	cpuInfo, er := cpu.Info()
	if er != nil {
		return nil, NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Get CPU info failed. err=(%v)", er).AddStack(nil)
	}
	// Get process info
	pid := os.Getpid()
	p, er := process.NewProcess(int32(pid))
	if er != nil {
		return nil, NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Get process info failed. err=(%v)", er).AddStack(nil)
	}

	return &AppRuntimeInfo{
		cpuInfo: cpuInfo,
		process: p,
	}, nil
}

// CPU

func (a *AppRuntimeInfo) CPUInfo() []cpu.InfoStat {
	return a.cpuInfo
}

func (a *AppRuntimeInfo) CPUUsage() (float64, *Error) {
	cpuPercent, er := a.process.CPUPercent()
	if er != nil {
		return 0.0, NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Get CPU percent failed. err=(%v)", er).AddStack(nil)
	}
	return cpuPercent, nil
}

// Memory

func (a *AppRuntimeInfo) MemoryInfo() (*process.MemoryInfoStat, *Error) {
	// Get detailed memory info
	memInfo, er := a.process.MemoryInfo()
	if er != nil {
		return nil, NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Get memory info failed. err=(%v)", er).AddStack(nil)
	}
	return memInfo, nil
}

func (a *AppRuntimeInfo) MemoryUsage() (real, virtual uint64, err *Error) {
	memInfo, er := a.process.MemoryInfo()
	if er != nil {
		return 0, 0, NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Get memory usage failed. err=(%v)", er).AddStack(nil)
	}
	return memInfo.RSS, memInfo.VMS, nil
}

func (a *AppRuntimeInfo) RuntimeMemoryInfo() *runtime.MemStats {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return &memStats
}

func (a *AppRuntimeInfo) RuntimeMemoryUsage() (alloc, sys uint64, err *Error) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	return memStats.Alloc, memStats.Sys, nil
}

// PProf

func (a *AppRuntimeInfo) CPUStartProfile() (string, *Error) {
	a.profileMutex.Lock()
	defer a.profileMutex.Unlock()

	if a.cpuProfile != nil {
		return "", NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: CPU profile already exists.").AddStack(nil)
	}

	// Open a file to write the profile to
	f, er := os.Create(fmt.Sprintf("cpu-%s.pprof", time.Now().Format("20060102-150405")))
	if er != nil {
		return "", NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Create CPU profile failed. err=(%v)", er).AddStack(nil)
	}
	absPath, er := filepath.Abs(f.Name())
	if er != nil {
		f.Close()
		return "", NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Get absolute path failed. err=(%v)", er).AddStack(nil)
	}
	if er := pprof.StartCPUProfile(f); er != nil {
		f.Close()
		return "", NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Start CPU profile failed. err=(%v)", er).AddStack(nil)
	}
	a.cpuProfile = f

	return absPath, nil
}

func (a *AppRuntimeInfo) CPUStopProfile() *Error {
	a.profileMutex.Lock()
	defer a.profileMutex.Unlock()

	if a.cpuProfile == nil {
		return NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: CPU profile not exists.").AddStack(nil)
	}

	// Write the CPU profile
	pprof.StopCPUProfile()

	// Close file
	if er := a.cpuProfile.Close(); er != nil {
		return NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Close CPU profile failed. err=(%v)", er).AddStack(nil)
	}
	a.cpuProfile = nil

	return nil
}

func (a *AppRuntimeInfo) MemoryHeapProfile() (string, *Error) {
	a.profileMutex.Lock()
	defer a.profileMutex.Unlock()

	// Open a file to write the profile to
	f, er := os.Create(fmt.Sprintf("mem-%s.pprof", time.Now().Format("20060102-150405")))
	if er != nil {
		return "", NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Create Memory profile failed. err=(%v)", er).AddStack(nil)
	}
	defer f.Close()
	absPath, er := filepath.Abs(f.Name())
	if er != nil {
		return "", NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Get absolute path failed. err=(%v)", er).AddStack(nil)
	}

	// Write the memory profile
	if er := pprof.WriteHeapProfile(f); er != nil {
		return "", NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Write Memory profile failed. err=(%v)", er).AddStack(nil)
	}

	return absPath, nil
}

func (a *AppRuntimeInfo) TraceStartProfile() *Error {
	a.profileMutex.Lock()
	defer a.profileMutex.Unlock()

	if a.traceProfile != nil {
		return NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Trace profile already exists.").AddStack(nil)
	}

	// Open a file to write the profile to
	f, er := os.Create(fmt.Sprintf("trace-%s.pprof", time.Now().Format("20060102-150405")))
	if er != nil {
		return NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Create Trace profile failed. err=(%v)", er).AddStack(nil)
	}
	// Write the trace profile
	if er := trace.Start(f); er != nil {
		f.Close()
		return NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Write Trace profile failed. err=(%v)", er).AddStack(nil)
	}
	a.traceProfile = f

	return nil
}

func (a *AppRuntimeInfo) TraceStopProfile() *Error {
	a.profileMutex.Lock()
	defer a.profileMutex.Unlock()

	if a.traceProfile == nil {
		return NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Trace profile not exists.").AddStack(nil)
	}

	// Stop the trace
	trace.Stop()

	// Close file
	if er := a.traceProfile.Close(); er != nil {
		return NewErrorf(ErrFrameworkRuntimeTrouble, "AppRuntimeInfo: Close Trace profile failed. err=(%v)", er).AddStack(nil)
	}
	a.traceProfile = nil

	return nil
}

// Util

func (a *AppRuntimeInfo) BitToMBit(b uint64) uint64 {
	return b / 1024 / 1024
}

func (a *AppRuntimeInfo) GarageCollect() {
	runtime.GC()
}
