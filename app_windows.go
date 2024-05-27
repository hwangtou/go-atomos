//go:build windows

package go_atomos

import "syscall"

func createSysProcAttr() *syscall.SysProcAttr {
	// Windows-specific SysProcAttr options can be set here if needed.
	return &syscall.SysProcAttr{}
}
