package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var ErrOwned = errors.New("project is already owned by another Tracelet process")

type Lock struct {
	file *os.File
	once sync.Once
	err  error
}

// Acquire uses a persistent lock file; removing it would let two processes lock different inodes.
func Acquire(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create project directory: %w", err)
	}
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("project lock must be a regular file")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open project lock: %w", err)
	}
	if err := takeLock(f); err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Lock{file: f}, nil
}

func (lock *Lock) Close() error {
	lock.once.Do(func() { lock.err = errors.Join(releaseLock(lock.file), lock.file.Close()) })
	return lock.err
}
