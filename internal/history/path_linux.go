//go:build linux

package history

import (
	"fmt"
	"syscall"
)

func validateLocalFilesystem(path string) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return fmt.Errorf("%w: inspect filesystem: %v", ErrUnsafePath, err)
	}
	// Linux has no MNT_LOCAL equivalent. Reject the stable magic values for
	// network-backed filesystems supported by the initial product boundary.
	switch uint64(stat.Type) {
	case 0x00006969, // NFS
		0xff534d42, // CIFS
		0x0000517b, // SMB
		0x5346414f, // AFS
		0x73757245, // CODA
		0x0000564c: // NCP
		return fmt.Errorf("%w: storage must use a local filesystem", ErrUnsafePath)
	default:
		return nil
	}
}
