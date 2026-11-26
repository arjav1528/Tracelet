//go:build !darwin && !linux && !windows

package session

import (
	"errors"
	"os"
)

func takeLock(_ *os.File) error {
	return errors.New("project ownership is not supported on this operating system")
}
func releaseLock(_ *os.File) error { return nil }
