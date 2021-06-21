package go_atomos

import "errors"

// Runnable

type CosmosRunnable struct {
	terminateSign chan string
	defines map[string]*ElementDefine
	script Script
}

func (r *CosmosRunnable) AddElement(define *ElementDefine) *CosmosRunnable {
	if r.defines == nil {
		r.defines = map[string]*ElementDefine{}
	}
	r.defines[define.Config.Name] = define
	return r
}

func (r *CosmosRunnable) SetScript(script Script) *CosmosRunnable {
	r.script = script
	return r
}

func (r *CosmosRunnable) Check() error {
	if r.defines == nil {
		// todo
		return errors.New("not add element yet")
	}
	if r.script == nil {
		// todo
		return errors.New("not set script yet")
	}
	return nil
}

// Dynamic Runnable

type CosmosDynamicRunnable struct {
	path string
	loaded bool
	runnable CosmosRunnable
}
