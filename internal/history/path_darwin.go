//go:build darwin

package history

import (
	"fmt"
	"syscall"
)

const darwinMountLocal = 0x00001000

func validateLocalFilesystem(path string) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return fmt.Errorf("%w: inspect filesystem: %v", ErrUnsafePath, err)
	}
	if stat.Flags&darwinMountLocal == 0 {
		return fmt.Errorf("%w: storage must use a local filesystem", ErrUnsafePath)
	}
	return nil
}
