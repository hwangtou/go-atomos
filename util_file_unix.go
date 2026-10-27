//go:build !windows

package atomos

import (
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strconv"
	"syscall"
)

const (
	utilFileEnsureDirectoryFilePerm = os.FileMode(0664)
)

type utilFileMode struct {
	Read, Write, Execute bool
}

//func UtilFileAccess(filepath string, perm os.FileMode) (err *Error) {
//	panic("")
//}

func utilFileEnsureDirectory(dir string, perm os.FileMode, ensureWritable bool) *Error {
	pathStat, er := os.Stat(dir)
	if er != nil {
		if !os.IsNotExist(er) {
			return NewErrorf(ErrCodeUtilFileEnsureDirectoryFailed, "utilFileEnsureDirectory: Failed to stat directory. dir=(%s),err=(%v)", dir, er).AddStack(nil)
		}
		if err := os.MkdirAll(dir, perm); err != nil {
			return NewErrorf(ErrCodeUtilFileEnsureDirectoryFailed, "utilFileEnsureDirectory: Failed to create directory. dir=(%s),err=(%v)", dir, err).AddStack(nil)
		}
	} else if !pathStat.IsDir() {
		return NewErrorf(ErrCodeUtilFileEnsureDirectoryFailed, "utilFileEnsureDirectory: utilPath is not directory. dir=(%s)", dir).AddStack(nil)
	}

	if ensureWritable {
		randStrGen := newUtilStringRandomStringGenerator()
		logTestPath := ""
		for i := 0; i < 100; i++ {
			logTestPath = path.Join(dir, "test_"+randStrGen.RandomString(10))
			if _, er = os.Stat(logTestPath); os.IsNotExist(er) {
				break
			}
		}
		if logTestPath == "" {
			return NewErrorf(ErrCodeUtilFileEnsureDirectoryFailed, "utilFileEnsureDirectory: Cannot create test file. dir=(%s)", dir).AddStack(nil)
		}
		if er = os.WriteFile(logTestPath, []byte{}, utilFileEnsureDirectoryFilePerm); er != nil {
			return NewErrorf(ErrCodeUtilFileEnsureDirectoryFailed, "utilFileEnsureDirectory: Cannot write test file. dir=(%s),err=(%v)", dir, er).AddStack(nil)
		}
		if er = os.Remove(logTestPath); er != nil {
			return NewErrorf(ErrCodeUtilFileEnsureDirectoryFailed, "utilFileEnsureDirectory: Cannot remove test file. dir=(%s),err=(%v)", dir, er).AddStack(nil)
		}
	}
	return nil
}

func utilFileFileExist(file string) (bool, *Error) {
	pathStat, er := os.Stat(file)
	if er != nil {
		if os.IsNotExist(er) {
			return false, nil
		}
		return false, NewErrorf(ErrCodeUtilFileExistCheckFailed, "utilFileFileExist: Failed to stat file. file=(%s),err=(%v)", file, er).AddStack(nil)
	}
	if pathStat.IsDir() {
		return false, NewErrorf(ErrCodeUtilFileExistCheckFailed, "utilFileFileExist: utilPath is directory. file=(%s)", file).AddStack(nil)
	}
	return true, nil
}

func utilFileGetDirectorySize(dir string) (int64, *Error) {
	var size int64
	er := filepath.Walk(dir, func(path string, info os.FileInfo, er error) error {
		if er != nil {
			return er
		}
		size += info.Size()
		return nil
	})
	if er != nil {
		return 0, NewErrorf(ErrCodeUtilFileGetDirectorySizeFailed, "utilFileGetDirectorySize: Failed to get directory size. dir=(%s),err=(%v)", dir, er).AddStack(nil)
	}
	return size, nil
}

// utilPath

type utilPath struct {
	os.FileInfo
	path string
}

func newUtilPath(path string) *utilPath {
	return &utilPath{path: path}
}

func (p *utilPath) Refresh() *Error {
	stat, er := os.Stat(p.path)
	if er != nil {
		if !os.IsNotExist(er) {
			return NewErrorf(ErrCodeUtilOSStatError, "utilPath: Get os state error. err=(%v)", er).AddStack(nil)
		}
	}
	p.FileInfo = stat
	return nil
}

func (p *utilPath) GetPath() string {
	return p.path
}

func (p *utilPath) CreateDirectoryIfNotExist(u *user.User, g *user.Group, perm os.FileMode) *Error {
	// If it is not exists, create it. If it is existing and not a directory, return failed.
	if err := p.Refresh(); err != nil {
		return err.AddStack(nil)
	}
	if !p.Exist() {
		if err := p.MakeDirectory(perm); err != nil {
			return err.AddStack(nil)
		}
		if err := p.ChangeOwnerAndMode(u, g, perm); err != nil {
			return err.AddStack(nil)
		}
	} else if !p.IsDir() {
		return NewError(ErrCodeUtilPathShouldBeDirectory, "utilPath: utilPath should be a directory.").AddStack(nil)
	} else {
		// Check its owner and mode.
		if err := p.ConfirmOwnerAndMode(g, perm); err != nil {
			return err.AddStack(nil)
		}
	}
	return nil
}

func (p *utilPath) CheckDirectoryOwnerAndMode(u *user.User, perm os.FileMode) *Error {
	if er := p.Refresh(); er != nil {
		return er.AddStack(nil)
	}
	if !p.Exist() {
		return NewError(ErrCodeUtilDirectoryNotExist, "utilPath: Directory not exists.").AddStack(nil)
	}
	// Owner
	gidList, er := u.GroupIds()
	if er != nil {
		return NewErrorf(ErrCodeUtilGetUserGroupIDsFailed, "utilPath: Get user group id list failed. err=(%v)", er).AddStack(nil)
	}
	switch stat := p.Sys().(type) {
	case *syscall.Stat_t:
		groupOwner := false
		statGid := strconv.FormatUint(uint64(stat.Gid), 10)
		for _, gid := range gidList {
			if gid == statGid {
				groupOwner = true
				break
			}
		}
		if !groupOwner {
			return NewError(ErrCodeUtilUsersGroupsHaveNotOwnedDirectory, "utilPath: User's groups have not owned directory.").AddStack(nil)
		}
	default:
		return NewErrorf(ErrCodeUtilNotSupportedOS, "utilPath: Not supported OS.").AddStack(nil)
	}
	// Mode
	if p.Mode().Perm() != perm {
		return NewError(ErrCodeUtilFileModePermNotMatch, "utilPath: File mode perm not match.").AddStack(nil)
	}
	return nil
}

func (p *utilPath) Exist() bool {
	return p.FileInfo != nil
}

func (p *utilPath) MakeDirectory(mode os.FileMode) *Error {
	if er := os.Mkdir(p.path, mode); er != nil {
		return NewErrorf(ErrCodeUtilFileMakeDirectoryFailed, "utilPath: Make directory failed. err=(%v)", er).AddStack(nil)
	}
	return nil
}

func (p *utilPath) ListDirectory() ([]*utilPath, *Error) {
	if !p.IsDir() {
		return nil, NewError(ErrCodeUtilPathShouldBeDirectory, "utilPath: Can only list directory.").AddStack(nil)
	}
	files, er := os.ReadDir(p.path)
	if er != nil {
		return nil, NewErrorf(ErrCodeUtilReadDirectoryFailed, "utilPath: Read dir failed. err=(%v)", er).AddStack(nil)
	}
	paths := make([]*utilPath, 0, len(files))
	for _, file := range files {
		filePath := newUtilPath(path.Join(p.path, file.Name()))
		if err := filePath.Refresh(); err == nil {
			paths = append(paths, filePath)
		}
	}
	return paths, nil
}

func (p *utilPath) ChangeOwnerAndMode(u *user.User, group *user.Group, perm os.FileMode) *Error {
	gid, er := strconv.ParseInt(group.Gid, 10, 64)
	if er != nil {
		return NewErrorf(ErrCodeUtilFileChangeOwnerAndModeFailed, "utilPath: Parse group gid failed. err=(%v)", er).AddStack(nil)
	}
	uid, er := strconv.ParseInt(u.Uid, 10, 64)
	if er != nil {
		return NewErrorf(ErrCodeUtilFileChangeOwnerAndModeFailed, "utilPath: Parse uid failed. err=(%v)", er).AddStack(nil)
	}
	if er = os.Chown(p.path, int(uid), int(gid)); er != nil {
		return NewErrorf(ErrCodeUtilFileChangeOwnerAndModeFailed, "utilPath: Change owner failed. err=(%v)", er).AddStack(nil)
	}
	if er = os.Chmod(p.path, perm); er != nil {
		return NewErrorf(ErrCodeUtilFileChangeOwnerAndModeFailed, "utilPath: Change mode failed. err=(%v)", er).AddStack(nil)
	}
	return nil
}

func (p *utilPath) ConfirmOwnerAndMode(group *user.Group, perm os.FileMode) *Error {
	// Owner
	switch stat := p.Sys().(type) {
	case *syscall.Stat_t:
		if strconv.FormatUint(uint64(stat.Gid), 10) != group.Gid {
			return NewError(ErrCodeUtilFileConfirmOwnerAndModeFailed, "utilPath: Parse group gid failed.").AddStack(nil)
		}
	default:
		return NewError(ErrCodeUtilFileConfirmOwnerAndModeFailed, "utilPath: Not supported OS.").AddStack(nil)
	}
	// Mode
	if p.Mode().Perm() != perm {
		return NewError(ErrCodeUtilFileConfirmOwnerAndModeFailed, "utilPath: File mode perm not match.").AddStack(nil)
	}
	return nil
}

func (p *utilPath) CreateFileIfNotExist(buf []byte, perm os.FileMode) *Error {
	if p.Exist() {
		return nil
	}
	er := os.WriteFile(p.path, buf, perm)
	if er != nil {
		return NewErrorf(ErrCodeUtilCreateFileFailed, "utilPath: Create file failed. err=(%v)", er).AddStack(nil)
	}
	return nil
}
