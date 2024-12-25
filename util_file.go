package go_atomos

import (
	"io/ioutil"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strconv"
	"syscall"
)

const (
	UtilFileEnsureDirectoryFilePerm = os.FileMode(0664)
)

func UtilFileEnsureDirectory(dir string, perm os.FileMode, ensureWritable bool) *Error {
	pathStat, er := os.Stat(dir)
	if er != nil {
		if !os.IsNotExist(er) {
			return NewErrorf(ErrUtilFileEnsureDirectoryFailed, "UtilFileEnsureDirectory: Failed to stat directory. dir=(%s),err=(%v)", dir, er).AddStack(nil)
		}
		if err := os.MkdirAll(dir, perm); err != nil {
			return NewErrorf(ErrUtilFileEnsureDirectoryFailed, "UtilFileEnsureDirectory: Failed to create directory. dir=(%s),err=(%v)", dir, err).AddStack(nil)
		}
	} else if !pathStat.IsDir() {
		return NewErrorf(ErrUtilFileEnsureDirectoryFailed, "UtilFileEnsureDirectory: Path is not directory. dir=(%s)", dir).AddStack(nil)
	}

	if ensureWritable {
		randStrGen := NewUtilStringRandomStringGenerator()
		logTestPath := ""
		for i := 0; i < 100; i++ {
			logTestPath = path.Join(dir, "test_"+randStrGen.RandomString(10))
			if _, er = os.Stat(logTestPath); os.IsNotExist(er) {
				break
			}
		}
		if logTestPath == "" {
			return NewErrorf(ErrUtilFileEnsureDirectoryFailed, "UtilFileEnsureDirectory: Cannot create test file. dir=(%s)", dir).AddStack(nil)
		}
		if er = os.WriteFile(logTestPath, []byte{}, UtilFileEnsureDirectoryFilePerm); er != nil {
			return NewErrorf(ErrUtilFileEnsureDirectoryFailed, "UtilFileEnsureDirectory: Cannot write test file. dir=(%s),err=(%v)", dir, er).AddStack(nil)
		}
		if er = os.Remove(logTestPath); er != nil {
			return NewErrorf(ErrUtilFileEnsureDirectoryFailed, "UtilFileEnsureDirectory: Cannot remove test file. dir=(%s),err=(%v)", dir, er).AddStack(nil)
		}
	}
	return nil
}

func UtilFileFileExist(file string) (bool, *Error) {
	pathStat, er := os.Stat(file)
	if er != nil {
		if os.IsNotExist(er) {
			return false, nil
		}
		return false, NewErrorf(ErrUtilFileFileExistFailed, "UtilFileFileExist: Failed to stat file. file=(%s),err=(%v)", file, er).AddStack(nil)
	}
	if pathStat.IsDir() {
		return false, NewErrorf(ErrUtilFileFileExistFailed, "UtilFileFileExist: Path is directory. file=(%s)", file).AddStack(nil)
	}
	return true, nil
}

func UtilFileGetDirectorySize(dir string) (int64, *Error) {
	var size int64
	er := filepath.Walk(dir, func(path string, info os.FileInfo, er error) error {
		if er != nil {
			return er
		}
		size += info.Size()
		return nil
	})
	if er != nil {
		return 0, NewErrorf(ErrUtilFileGetDirectorySizeFailed, "UtilFileGetDirectorySize: Failed to get directory size. dir=(%s),err=(%v)", dir, er).AddStack(nil)
	}
	return size, nil
}

// Path

type Path struct {
	os.FileInfo
	path string
}

func NewPath(path string) *Path {
	return &Path{path: path}
}

func (p *Path) Refresh() *Error {
	stat, er := os.Stat(p.path)
	if er != nil {
		if !os.IsNotExist(er) {
			return NewErrorf(ErrUtilOSStatError, "Path: Get os state error. err=(%v)", er).AddStack(nil)
		}
	}
	p.FileInfo = stat
	return nil
}

func (p *Path) GetPath() string {
	return p.path
}

func (p *Path) CreateDirectoryIfNotExist(u *user.User, g *user.Group, perm os.FileMode) *Error {
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
		return NewError(ErrUtilPathShouldBeDirectory, "Path: Path should be a directory.").AddStack(nil)
	} else {
		// Check its owner and mode.
		if err := p.ConfirmOwnerAndMode(g, perm); err != nil {
			return err.AddStack(nil)
		}
	}
	return nil
}

func (p *Path) CheckDirectoryOwnerAndMode(u *user.User, perm os.FileMode) *Error {
	if er := p.Refresh(); er != nil {
		return er.AddStack(nil)
	}
	if !p.Exist() {
		return NewError(ErrUtilDirectoryNotExist, "Path: Directory not exists.").AddStack(nil)
	}
	// Owner
	gidList, er := u.GroupIds()
	if er != nil {
		return NewErrorf(ErrUtilGetUserGroupIDsFailed, "Path: Get user group id list failed. err=(%v)", er).AddStack(nil)
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
			return NewError(ErrUtilUsersGroupsHaveNotOwnedDirectory, "Path: User's groups have not owned directory.").AddStack(nil)
		}
	default:
		return NewErrorf(ErrUtilNotSupportedOS, "Path: Not supported OS.").AddStack(nil)
	}
	// Mode
	if p.Mode().Perm() != perm {
		return NewError(ErrUtilFileModePermNotMatch, "Path: File mode perm not match.").AddStack(nil)
	}
	return nil
}

func (p *Path) Exist() bool {
	return p.FileInfo != nil
}

func (p *Path) MakeDirectory(mode os.FileMode) *Error {
	if er := os.Mkdir(p.path, mode); er != nil {
		return NewErrorf(ErrUtilFileMakeDirectoryFailed, "Path: Make directory failed. err=(%v)", er).AddStack(nil)
	}
	return nil
}

func (p *Path) ListDirectory() ([]*Path, *Error) {
	if !p.IsDir() {
		return nil, NewError(ErrUtilPathShouldBeDirectory, "Path: Can only list directory.").AddStack(nil)
	}
	files, er := os.ReadDir(p.path)
	if er != nil {
		return nil, NewErrorf(ErrUtilReadDirectoryFailed, "Path: Read dir failed. err=(%v)", er).AddStack(nil)
	}
	paths := make([]*Path, 0, len(files))
	for _, file := range files {
		filePath := NewPath(path.Join(p.path, file.Name()))
		if err := filePath.Refresh(); err == nil {
			paths = append(paths, filePath)
		}
	}
	return paths, nil
}

func (p *Path) ChangeOwnerAndMode(u *user.User, group *user.Group, perm os.FileMode) *Error {
	gid, er := strconv.ParseInt(group.Gid, 10, 64)
	if er != nil {
		return NewErrorf(ErrUtilFileChangeOwnerAndModeFailed, "Path: Parse group gid failed. err=(%v)", er).AddStack(nil)
	}
	uid, er := strconv.ParseInt(u.Uid, 10, 64)
	if er != nil {
		return NewErrorf(ErrUtilFileChangeOwnerAndModeFailed, "Path: Parse uid failed. err=(%v)", er).AddStack(nil)
	}
	if er = os.Chown(p.path, int(uid), int(gid)); er != nil {
		return NewErrorf(ErrUtilFileChangeOwnerAndModeFailed, "Path: Change owner failed. err=(%v)", er).AddStack(nil)
	}
	if er = os.Chmod(p.path, perm); er != nil {
		return NewErrorf(ErrUtilFileChangeOwnerAndModeFailed, "Path: Change mode failed. err=(%v)", er).AddStack(nil)
	}
	return nil
}

func (p *Path) ConfirmOwnerAndMode(group *user.Group, perm os.FileMode) *Error {
	// Owner
	switch stat := p.Sys().(type) {
	case *syscall.Stat_t:
		if strconv.FormatUint(uint64(stat.Gid), 10) != group.Gid {
			return NewError(ErrUtilFileConfirmOwnerAndModeFailed, "Path: Parse group gid failed.").AddStack(nil)
		}
	default:
		return NewError(ErrUtilFileConfirmOwnerAndModeFailed, "Path: Not supported OS.").AddStack(nil)
	}
	// Mode
	if p.Mode().Perm() != perm {
		return NewError(ErrUtilFileConfirmOwnerAndModeFailed, "Path: File mode perm not match.").AddStack(nil)
	}
	return nil
}

func (p *Path) CreateFileIfNotExist(buf []byte, perm os.FileMode) *Error {
	if p.Exist() {
		return nil
	}
	er := ioutil.WriteFile(p.path, buf, perm)
	if er != nil {
		return NewErrorf(ErrUtilCreateFileFailed, "Path: Create file failed. err=(%v)", er).AddStack(nil)
	}
	return nil
}
