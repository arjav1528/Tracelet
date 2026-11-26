//go:build !darwin && !linux && !windows

package supervisor

import (
	"errors"
	"os"
	"os/exec"
)

type processTree struct{}

func prepare(_ *exec.Cmd) (*processTree, error) {
	return nil, errors.New("process supervision is not supported on this operating system")
}
func (*processTree) started(_ *exec.Cmd) error       { return nil }
func (*processTree) close() error                    { return nil }
func (*processTree) signal(_ int, _ os.Signal) error { return nil }
func (*processTree) kill(_ int) error                { return nil }
func terminationSignal() os.Signal                   { return os.Interrupt }
func processExitCode(state *os.ProcessState) int     { return state.ExitCode() }
