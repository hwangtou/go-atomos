package atomos

import "os"

func utilFileCheckWritable(dir string) error {
	file, err := os.CreateTemp(dir, ".atomos-write-test-*")
	if err != nil {
		return err
	}
	name := file.Name()
	closeErr := file.Close()
	removeErr := os.Remove(name)
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func utilFileWriteFileNoFollow(path string, data []byte, perm os.FileMode) error {
	file, err := utilFileOpenNoFollow(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
