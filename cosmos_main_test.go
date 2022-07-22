package go_atomos

import (
	"testing"
	"time"
)

func TestCosmosMain(t *testing.T) {
	process := newTestFakeCosmosProcess(t)
	runnable := newTestFakeRunnable(t)
	main := newCosmosMain(process, runnable)
	main.Log().Info("TestCosmosMain")

	err := main.onceLoad(runnable)
	if err != nil {
		main.Log().Info("Main: Reload failed, err=(%v)", err)
		return
	}

	go func() {
		time.Sleep(100 * time.Millisecond)

		err := main.pushReloadMail(nil, runnable, 2)
		main.Log().Info("Main: Reload, err=(%v)", err)
	}()

	// Execute newRunnable script after reloading all elements and atomos.
	go func() {
		if err = func(runnable *CosmosRunnable) (err *ErrorInfo) {
			defer func() {
				if r := recover(); r != nil {
					err = NewErrorf(ErrMainFnReloadFailed, "Main: Reload script CRASH! reason=(%s)", r)
					main.Log().Fatal(err.Message)
				}
			}()
			runnable.mainScript(main, main.mainKillCh)
			return
		}(runnable); err != nil {
			main.Log().Fatal("Main: Load failed, err=(%v)", err)
		}
	}()

	time.Sleep(200 * time.Millisecond)
	err = main.pushKillMail(nil, true)
	main.Log().Info("Main: Exit, err=(%v)", err)

	// TODO: Reload

	time.Sleep(100 * time.Millisecond)
}
