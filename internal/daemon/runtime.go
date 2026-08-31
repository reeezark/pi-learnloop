package daemon

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type runtimeLock struct {
	file *os.File
}

type runtimeDescriptor struct {
	SchemaVersion   int    `json:"schema_version"`
	ProtocolVersion int    `json:"protocol_version"`
	InstanceID      string `json:"instance_id"`
	PID             int    `json:"pid"`
	BaseURL         string `json:"base_url"`
	StartedAt       string `json:"started_at"`
}

func prepareStateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("daemon: create state directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("daemon: inspect state directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("daemon: state directory is not a real directory")
	}
	if info.Mode().Perm() != 0o700 {
		return fmt.Errorf("daemon: state directory permissions are %04o, want 0700", info.Mode().Perm())
	}
	if err := validateOwner(info); err != nil {
		return fmt.Errorf("daemon: state directory: %w", err)
	}
	return nil
}

func acquireRuntimeLock(stateDir string) (*runtimeLock, error) {
	path := filepath.Join(stateDir, "daemon.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrExist) {
		if err := validateProtectedFile(path); err != nil {
			return nil, fmt.Errorf("daemon: runtime lock: %w", err)
		}
		file, err = os.OpenFile(path, os.O_RDWR, 0)
	}
	if err != nil {
		return nil, fmt.Errorf("daemon: open runtime lock: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, fmt.Errorf("daemon: protect runtime lock: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("daemon: inspect opened runtime lock: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		file.Close()
		return nil, errors.New("daemon: opened runtime lock is not a protected regular file")
	}
	if err := validateOwner(info); err != nil {
		file.Close()
		return nil, fmt.Errorf("daemon: opened runtime lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrAlreadyRunning
		}
		return nil, fmt.Errorf("daemon: acquire runtime lock: %w", err)
	}
	return &runtimeLock{file: file}, nil
}

func (lock *runtimeLock) release() {
	_ = syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	_ = lock.file.Close()
}

func validateProtectedFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("path is not a regular file")
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("permissions are %04o, want 0600", info.Mode().Perm())
	}
	return validateOwner(info)
}

func validateOwner(info os.FileInfo) error {
	statistics, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("owner information is unavailable")
	}
	if int(statistics.Uid) != os.Geteuid() {
		return fmt.Errorf("owner is UID %d, want effective UID %d", statistics.Uid, os.Geteuid())
	}
	return nil
}

func randomID(size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func publishDescriptor(stateDir string, descriptor runtimeDescriptor) error {
	content, err := json.Marshal(descriptor)
	if err != nil {
		return fmt.Errorf("daemon: encode runtime descriptor: %w", err)
	}
	if err := writeAtomic(filepath.Join(stateDir, "daemon.json"), content); err != nil {
		return fmt.Errorf("daemon: publish runtime descriptor: %w", err)
	}
	return nil
}

func publishToken(stateDir, token string) error {
	if err := writeAtomic(filepath.Join(stateDir, "daemon.token"), []byte(token)); err != nil {
		return fmt.Errorf("daemon: publish instance token: %w", err)
	}
	return nil
}

func writeAtomic(path string, content []byte) error {
	if err := validateReplaceTarget(path); err != nil {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".pi-learnloop-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	directoryHandle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer directoryHandle.Close()
	return directoryHandle.Sync()
}

func validateReplaceTarget(path string) error {
	if err := validateProtectedFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return nil
}

func removeRuntimeFiles(stateDir, instanceID string) {
	path := filepath.Join(stateDir, "daemon.json")
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var descriptor runtimeDescriptor
	if err := json.Unmarshal(content, &descriptor); err != nil || descriptor.InstanceID != instanceID {
		return
	}
	_ = os.Remove(path)
	_ = os.Remove(filepath.Join(stateDir, "daemon.token"))
}
