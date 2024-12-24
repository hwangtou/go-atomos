package go_atomos

import (
	"os"
	"path"
	"path/filepath"
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
