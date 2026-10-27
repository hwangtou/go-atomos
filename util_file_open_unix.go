//go:build !windows

package atomos

import (
	"os"
	"syscall"
)

func utilFileOpenNoFollow(path string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flag|syscall.O_NOFOLLOW, perm)
}
