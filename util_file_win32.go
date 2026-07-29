//go:build windows

package atomos

import (
	"os"
	"os/user"
)

func UtilFileEnsureDirectory(dir string, perm os.FileMode, ensureWritable bool) *Error {
	return NewErrorf(ErrUtilFileEnsureDirectoryFailed, "UtilFileEnsureDirectory: Not implemented on windows platform. dir=(%s)", dir).AddStack(nil)
}

func UtilFileFileExist(file string) (bool, *Error) {
	return false, NewErrorf(ErrUtilFileFileExistFailed, "UtilFileFileExist: Not implemented on windows platform. file=(%s)", file).AddStack(nil)
}

func UtilFileGetDirectorySize(dir string) (int64, *Error) {
	return 0, NewErrorf(ErrUtilFileGetDirectorySizeFailed, "UtilFileGetDirectorySize: Not implemented on windows platform. dir=(%s)", dir).AddStack(nil)
}

// Path

type Path struct {
	path string
}

func NewPath(p string) *Path {
	return &Path{path: p}
}

func (p *Path) Refresh() *Error {
	return NewErrorf(ErrUtilOSStatError, "Path: Not implemented on windows platform. path=(%s)", p.path).AddStack(nil)
}

func (p *Path) GetPath() string {
	return p.path
}

func (p *Path) CreateDirectoryIfNotExist(u *user.User, g *user.Group, perm os.FileMode) *Error {
	return NewErrorf(ErrUtilFileEnsureDirectoryFailed, "Path: CreateDirectoryIfNotExist not implemented on windows platform. path=(%s)", p.path).AddStack(nil)
}

func (p *Path) CheckDirectoryOwnerAndMode(u *user.User, perm os.FileMode) *Error {
	return NewErrorf(ErrUtilFileEnsureDirectoryFailed, "Path: CheckDirectoryOwnerAndMode not implemented on windows platform. path=(%s)", p.path).AddStack(nil)
}

func (p *Path) Exist() bool {
	return false
}

func (p *Path) MakeDirectory(mode os.FileMode) *Error {
	return NewErrorf(ErrUtilFileEnsureDirectoryFailed, "Path: MakeDirectory not implemented on windows platform. path=(%s)", p.path).AddStack(nil)
}

func (p *Path) ListDirectory() ([]*Path, *Error) {
	return nil, NewErrorf(ErrUtilFileEnsureDirectoryFailed, "Path: ListDirectory not implemented on windows platform. path=(%s)", p.path).AddStack(nil)
}

func (p *Path) ChangeOwnerAndMode(u *user.User, group *user.Group, perm os.FileMode) *Error {
	return NewErrorf(ErrUtilFileEnsureDirectoryFailed, "Path: ChangeOwnerAndMode not implemented on windows platform. path=(%s)", p.path).AddStack(nil)
}

func (p *Path) ConfirmOwnerAndMode(group *user.Group, perm os.FileMode) *Error {
	return NewErrorf(ErrUtilFileEnsureDirectoryFailed, "Path: ConfirmOwnerAndMode not implemented on windows platform. path=(%s)", p.path).AddStack(nil)
}

func (p *Path) CreateFileIfNotExist(buf []byte, perm os.FileMode) *Error {
	return NewErrorf(ErrUtilFileEnsureDirectoryFailed, "Path: CreateFileIfNotExist not implemented on windows platform. path=(%s)", p.path).AddStack(nil)
}
