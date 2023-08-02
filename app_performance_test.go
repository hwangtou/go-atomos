package go_atomos

import (
	"testing"
	"time"
)

func TestNewAppRuntimeInfo(t *testing.T) {
	rt, err := NewAppRuntimeInfo()
	if err != nil {
		t.Error(err)
		return
	}
	go func() {
		for {
			_ = make([]byte, 1024*1024)
		}
	}()
	<-time.After(100 * time.Millisecond)

	t.Logf("CPU")
	t.Logf("CPUInfo: %s", rt.CPUInfo())
	for i := 0; i < 5; i++ {
		cpuPercent, err := rt.CPUUsage()
		if err != nil {
			t.Error(err)
			return
		}
		t.Logf("CPUUsage: %f", cpuPercent)

		<-time.After(1 * time.Second)
	}

	t.Logf("Memory")
	for i := 0; i < 5; i++ {
		memInfo, err := rt.MemoryInfo()
		if err != nil {
			t.Error(err)
			return
		}
		alloc, sys, err := rt.RuntimeMemoryUsage()
		if err != nil {
			t.Error(err)
			return
		}
		runtimeMemInfo := rt.RuntimeMemoryInfo()
		t.Logf("Alloc=(%d),Sys=(%d),MemoryInfo=(%+v),RuntimeMemoryInfo=(%+v)", alloc, sys, memInfo, runtimeMemInfo)

		<-time.After(1 * time.Second)
	}
	t.Logf("Runtime MemoryInfo: %+v", rt.RuntimeMemoryInfo())
}
