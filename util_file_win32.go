//go:build windows

package atomos

import (
	"os"
	"os/user"
	"path"
	"path/filepath"
)

// Windows 实现说明:
// Windows 没有 POSIX 的 uid/gid/文件权限位语义 (它用 ACL 模型), 故 unix 版里
// 基于 syscall.Stat_t 的属主/权限检查 (CheckDirectoryOwnerAndMode /
// ConfirmOwnerAndMode / ChangeOwnerAndMode 的 chown 部分) 在 Windows 上无对应概念。
// 这里对文件/目录的创建、存在性、列举等用跨平台 os API 真实实现; 涉及属主/权限的
// 检查降级为只校验存在性与目录类型 (权限相关直接放行), 让框架在 Windows 上能跑通。
// os/user 仅为与 unix 版签名一致而 import (CreateDirectoryIfNotExist 等保留 u/g 参数)。

const (
	// UtilFileEnsureDirectoryFilePerm 与 unix 版一致 (Windows 忽略权限位, 仅作占位)。
	UtilFileEnsureDirectoryFilePerm = os.FileMode(0664)
)

type UtilFileMode struct {
	Read, Write, Execute bool
}

// UtilFileEnsureDirectory 确保目录存在 (不存在则 MkdirAll 创建), ensureWritable 时
// 通过写一个临时文件再删除来验证可写。
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

// CreateDirectoryIfNotExist 创建目录。Windows 无 POSIX 属主/权限概念,
// owner (u/g) 与 perm 在此降级: 仅创建目录并校验类型, 跳过 chown/权限校验。
func (p *Path) CreateDirectoryIfNotExist(u *user.User, g *user.Group, perm os.FileMode) *Error {
	if err := p.Refresh(); err != nil {
		return err.AddStack(nil)
	}
	if !p.Exist() {
		if err := p.MakeDirectory(perm); err != nil {
			return err.AddStack(nil)
		}
		// Windows: ChangeOwnerAndMode 降级为 no-op (见下)。
		if err := p.ChangeOwnerAndMode(u, g, perm); err != nil {
			return err.AddStack(nil)
		}
	} else if !p.IsDir() {
		return NewError(ErrUtilPathShouldBeDirectory, "Path: Path should be a directory.").AddStack(nil)
	}
	// Windows: ConfirmOwnerAndMode 降级为 no-op。
	return nil
}

// CheckDirectoryOwnerAndMode Windows 降级: 仅校验目录存在, 跳过属主/权限检查。
func (p *Path) CheckDirectoryOwnerAndMode(u *user.User, perm os.FileMode) *Error {
	if er := p.Refresh(); er != nil {
		return er.AddStack(nil)
	}
	if !p.Exist() {
		return NewError(ErrUtilDirectoryNotExist, "Path: Directory not exists.").AddStack(nil)
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

// ChangeOwnerAndMode Windows 降级: chown 无对应概念, 仅尝试 chmod (Windows 上
// chmod 只保留只读位, 大多数情况 no-op)。不报错, 保证框架启动链路通过。
func (p *Path) ChangeOwnerAndMode(u *user.User, group *user.Group, perm os.FileMode) *Error {
	_ = os.Chmod(p.path, perm)
	return nil
}

// ConfirmOwnerAndMode Windows 降级: 无 POSIX 属主/权限位, 直接放行。
func (p *Path) ConfirmOwnerAndMode(group *user.Group, perm os.FileMode) *Error {
	return nil
}

func (p *Path) CreateFileIfNotExist(buf []byte, perm os.FileMode) *Error {
	if p.Exist() {
		return nil
	}
	er := os.WriteFile(p.path, buf, perm)
	if er != nil {
		return NewErrorf(ErrUtilCreateFileFailed, "Path: Create file failed. err=(%v)", er).AddStack(nil)
	}
	return nil
}
