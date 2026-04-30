//go:build darwin || dragonfly || freebsd || linux || nacl || netbsd || openbsd || solaris

package atomos

import "syscall"

func createSysProcAttr() *syscall.SysProcAttr {
	// Unix-specific SysProcAttr options can be set here if needed.
	return &syscall.SysProcAttr{Setsid: true}
}
