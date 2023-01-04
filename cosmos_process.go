package go_atomos

import (
	"sync"
)

// CosmosProcess
// 这个才是进程的主循环。

type CosmosProcess struct {
	mutex sync.RWMutex
	state CosmosProcessState
	main  *CosmosMain
}

type CosmosMainScript interface {
	OnStartup() *Error
	OnShutdown() *Error
}

type CosmosProcessState int

const (
	CosmosProcessStatePrepare  CosmosProcessState = 0
	CosmosProcessStateStartup  CosmosProcessState = 1
	CosmosProcessStateRunning  CosmosProcessState = 2
	CosmosProcessStateShutdown CosmosProcessState = 3
	CosmosProcessStateOff      CosmosProcessState = 4
)

var sharedCosmosProcess *CosmosProcess
var onceInitSharedCosmosProcess sync.Once

func SharedCosmosProcess() *CosmosProcess {
	return sharedCosmosProcess
}

func InitCosmosProcess(accessLogFn, errLogFn LoggingFn) {
	onceInitSharedCosmosProcess.Do(func() {
		sharedCosmosProcess = &CosmosProcess{}
		initSharedLoggingAtomos(accessLogFn, errLogFn)
		initCosmosMain(sharedCosmosProcess)
	})
}

func (p *CosmosProcess) Start(runnable *CosmosRunnable) *Error {
	// Check if in prepare state.
	if err := func() *Error {
		if p == nil {
			return NewError(ErrCosmosProcessHasNotInitialized, "CosmosProcess: Process has not initialized.").
				AddStack(nil)
		}
		p.mutex.Lock()
		defer p.mutex.Unlock()

		if p.state != CosmosProcessStatePrepare {
			return NewError(ErrCosmosProcessHasBeenStarted, "CosmosProcess: Process can only start once.").
				AddStack(nil)
		}
		p.state = CosmosProcessStateStartup
		return nil
	}(); err != nil {
		return err
	}

	// Starting Up.
	if err := func() (err *Error) {
		defer func() {
			if r := recover(); r != nil {
				err = NewError(ErrCosmosProcessOnStartupPanic, "CosmosProcess: Process startups panic.").
					AddPanicStack(p.main, 1, r)
			}
		}()
		if err = runnable.Check(); err != nil {
			return err.AddStack(nil)
		}
		if err = p.main.loadOnce(runnable); err != nil {
			return err.AddStack(nil)
		}
		if err = p.main.runnable.mainScript.OnStartup(); err != nil {
			p.mutex.Lock()
			p.state = CosmosProcessStateOff
			p.mutex.Unlock()
			return err
		}
		return nil
	}(); err != nil {
		return err
	}

	p.mutex.Lock()
	p.state = CosmosProcessStateRunning
	p.mutex.Unlock()

	return nil
}

func (p *CosmosProcess) Stop() *Error {
	if err := func() *Error {
		p.mutex.Lock()
		defer p.mutex.Unlock()
		switch p.state {
		case CosmosProcessStatePrepare:
			return NewError(ErrCosmosProcessCannotStopPrepareState, "CosmosProcess: Stopping process is preparing.").AddStack(nil)
		case CosmosProcessStateStartup:
			return NewError(ErrCosmosProcessCannotStopStartupState, "CosmosProcess: Stopping process is starting up.").AddStack(nil)
		case CosmosProcessStateRunning:
			p.state = CosmosProcessStateShutdown
			return nil
		case CosmosProcessStateShutdown:
			return NewError(ErrCosmosProcessCannotStopShutdownState, "CosmosProcess: Stopping process is shutting down.").AddStack(nil)
		case CosmosProcessStateOff:
			return NewError(ErrCosmosProcessCannotStopOffState, "CosmosProcess: Stopping process is halt.").AddStack(nil)
		}
		return NewError(ErrCosmosProcessInvalidState, "CosmosProcess: Stopping process is in invalid process state.").AddStack(nil)
	}(); err != nil {
		return err
	}

	// Starting Up.
	err := func() (err *Error) {
		defer func() {
			if r := recover(); r != nil {
				err = NewError(ErrCosmosProcessOnShutdownPanic, "CosmosProcess: Process shutdowns panic.").
					AddPanicStack(p.main, 1, r)
			}
		}()
		if err = p.main.runnable.mainScript.OnShutdown(); err != nil {
			return err
		}
		return p.main.pushKillMail(p.main, true, 0)
	}()

	p.mutex.Lock()
	p.state = CosmosProcessStateOff
	p.mutex.Unlock()

	if err != nil {
		return err
	}
	return nil
}

func (p *CosmosProcess) Self() *CosmosMain {
	return p.main
}
