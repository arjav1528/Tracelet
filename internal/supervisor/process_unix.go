//go:build darwin || linux

package supervisor

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

type processTree struct{}

func prepare(command *exec.Cmd) (*processTree, error) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return &processTree{}, nil
}
func (*processTree) started(_ *exec.Cmd) error { return nil }
func (*processTree) close() error              { return nil }
func (*processTree) signal(pid int, sig os.Signal) error {
	value, ok := sig.(syscall.Signal)
	if !ok {
		return errors.New("unsupported termination signal")
	}
	return groupSignal(pid, value)
}
func (*processTree) kill(pid int) error { return groupSignal(pid, syscall.SIGKILL) }
func groupSignal(pid int, sig syscall.Signal) error {
	err := syscall.Kill(-pid, sig)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
func terminationSignal() os.Signal { return syscall.SIGTERM }
func processExitCode(state *os.ProcessState) int {
	if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return state.ExitCode()
}
