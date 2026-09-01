//go:build darwin || linux

package history

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	dataDirectoryMode = 0o700
	databaseFileMode  = 0o600
	maxStoragePathLen = 4096
)

func prepareStorage(dataDir string) (string, error) {
	if dataDir == "" || !filepath.IsAbs(dataDir) || filepath.Clean(dataDir) != dataDir || len(dataDir) > maxStoragePathLen || strings.IndexByte(dataDir, 0) >= 0 {
		return "", fmt.Errorf("%w: data directory must be a clean absolute path", ErrUnsafePath)
	}
	if err := os.MkdirAll(dataDir, dataDirectoryMode); err != nil {
		return "", fmt.Errorf("%w: create data directory: %v", ErrUnsafePath, err)
	}
	if err := validateProtectedPath(dataDir, true, dataDirectoryMode); err != nil {
		return "", err
	}
	if err := validateLocalFilesystem(dataDir); err != nil {
		return "", err
	}

	databasePath := filepath.Join(dataDir, "history.db")
	file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, databaseFileMode)
	if err == nil {
		if closeErr := file.Close(); closeErr != nil {
			return "", fmt.Errorf("create history database: %w", closeErr)
		}
	} else if !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("create history database: %w", err)
	}
	if err := validateProtectedPath(databasePath, false, databaseFileMode); err != nil {
		return "", err
	}
	return databasePath, nil
}

func validateProtectedPath(path string, directory bool, wantMode os.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%w: inspect %q: %v", ErrUnsafePath, path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %q is a symbolic link", ErrUnsafePath, path)
	}
	if directory && !info.IsDir() {
		return fmt.Errorf("%w: %q is not a directory", ErrUnsafePath, path)
	}
	if !directory && !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %q is not a regular file", ErrUnsafePath, path)
	}
	if info.Mode().Perm() != wantMode {
		return fmt.Errorf("%w: %q has permissions %#o, want %#o", ErrUnsafePath, path, info.Mode().Perm(), wantMode)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("%w: %q is not owned by the current account", ErrUnsafePath, path)
	}
	if !directory && stat.Nlink != 1 {
		return fmt.Errorf("%w: %q must have exactly one hard link", ErrUnsafePath, path)
	}
	return nil
}
