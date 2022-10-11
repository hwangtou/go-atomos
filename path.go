package go_atomos

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"strconv"
	"syscall"
)

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
			return NewErrorf(ErrPathNotExist, "path is not exist, path=(%s),err=(%v)", p.path, er).AutoStack(nil, nil)
		}
	}
	p.FileInfo = stat
	return nil
}

func (p *Path) CreateDirectoryIfNotExist(u *user.User, g *user.Group, perm os.FileMode) *Error {
	// If it is not exists, create it. If it is exists and not a directory, return failed.
	if er := p.Refresh(); er != nil {
		return er.AutoStack(nil, nil)
	}
	if !p.Exist() {
		if er := p.MakeDir(perm); er != nil {
			return er.AutoStack(nil, nil)
		}
		if er := p.ChangeOwnerAndMode(u, g, perm); er != nil {
			return er
		}
	} else if !p.IsDir() {
		return NewError(ErrPathIsNotDirectory, "path should be directory").AutoStack(nil, nil)
	} else {
		// Check its owner and mode.
		if er := p.ConfirmOwnerAndMode(g, perm); er != nil {
			return er.AutoStack(nil, nil)
		}
	}
	return nil
}

func (p *Path) CheckDirectoryOwnerAndMode(u *user.User, perm os.FileMode) *Error {
	if er := p.Refresh(); er != nil {
		fmt.Println("Error: Refresh failed, err=", er)
		return er
	}
	if !p.Exist() {
		return NewErrorf(ErrPathNotExist, "path is not exist, path=(%s)", p.path).AutoStack(nil, nil)
	}
	// Owner
	gidList, er := u.GroupIds()
	if er != nil {
		return NewErrorf(ErrPathGetGroupIDsFailed, "path get group ids failed, err=(%v)", er).AutoStack(nil, nil)
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
			return NewError(ErrPathIsNotOwner, "user's group is not owned directory").AutoStack(nil, nil)
		}
	default:
		return NewError(ErrFrameworkPanic, "OS not supported").AutoStack(nil, nil)
	}
	// Mode
	if p.Mode().Perm() != perm {
		return NewErrorf(ErrPathPermNotMatch, "invalid mode, mode=(%v)", p.Mode().Perm())
	}
	return nil
}

func (p *Path) Exist() bool {
	return p.FileInfo != nil
}

func (p *Path) MakeDir(mode os.FileMode) *Error {
	if er := os.Mkdir(p.path, mode); er != nil {
		return NewErrorf(ErrPathMakeDir, "path make dir failed, path=(%s),err=(%v)", p.path, er).AutoStack(nil, nil)
	}
	return nil
}

func (p *Path) ChangeOwnerAndMode(u *user.User, group *user.Group, perm os.FileMode) *Error {
	gid, er := strconv.ParseInt(group.Gid, 10, 64)
	if er != nil {
		return NewErrorf(ErrPathGroupIDInvalid, "group id invalid, gidStr=(%s)", group.Gid).AutoStack(nil, nil)
	}
	uid, er := strconv.ParseInt(u.Uid, 10, 64)
	if er != nil {
		return NewErrorf(ErrPathUserIDInvalid, "user id invalid, uidStr=(%s)", u.Uid).AutoStack(nil, nil)
	}
	if er = os.Chown(p.path, int(uid), int(gid)); er != nil {
		return NewErrorf(ErrPathChangeOwnFailed, "path change owner failed, err=(%s)", er).AutoStack(nil, nil)
	}
	if er = os.Chmod(p.path, perm); er != nil {
		return NewErrorf(ErrPathChangeModeFailed, "path change mode failed, err=(%s)", er).AutoStack(nil, nil)
	}
	return nil
}

func (p *Path) ConfirmOwnerAndMode(group *user.Group, perm os.FileMode) *Error {
	// Owner
	switch stat := p.Sys().(type) {
	case *syscall.Stat_t:
		if strconv.FormatUint(uint64(stat.Gid), 10) != group.Gid {
			return NewErrorf(ErrPathIsNotOwner, "user is not path owner, gid=(%d)", group.Gid).AutoStack(nil, nil)
		}
	default:
		return NewError(ErrFrameworkPanic, "OS not supported").AutoStack(nil, nil)
	}

	// Mode
	if p.Mode().Perm() != perm {
		return NewErrorf(ErrPathPermNotMatch, "invalid mode, mode=(%v)", p.Mode().Perm())
	}
	return nil
}

func (p *Path) SaveFile(buf []byte, perm os.FileMode) *Error {
	er := ioutil.WriteFile(p.path, buf, perm)
	if er != nil {
		return NewErrorf(ErrPathSaveFileFailed, "path save file failed, err=(%v)", er).AutoStack(nil, nil)
	}
	return nil
}
