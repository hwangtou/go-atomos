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

	helper, err := main.newRunnableLoadingHelper(nil, runnable)
	if err != nil {
		main.Log().Fatal(err.Message)
		return
	}

	if err = main.trySpawningElements(helper); err != nil {
		main.Log().Fatal(err.Message)
		return
	}

	// NOTICE: Spawning might fail, it might cause reloads count increase, but actually reload failed. TODO
	// NOTICE: After spawning, reload won't stop, no matter what error happens.

	main.listenCert = helper.listenCert
	main.clientCert = helper.clientCert
	//main.remote = helper.remote

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
