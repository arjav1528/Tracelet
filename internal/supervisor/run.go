package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"time"
)

// Run keeps supervision active through graceful shutdown; callers close ingestion after it returns.
func Run(ctx context.Context, args, env []string, stdout, stderr io.Writer, grace time.Duration) (code int, err error) {
	if len(args) == 0 || args[0] == "" {
		return 1, errors.New("application command is required")
	}
	if grace <= 0 {
		return 1, errors.New("shutdown grace must be positive")
	}
	if err := ctx.Err(); err != nil {
		return 1, err
	}
	command := exec.Command(args[0], args[1:]...)
	command.Env = env
	command.Stdin = os.Stdin
	command.Stdout, command.Stderr = stdout, stderr
	// Bound inherited output pipes even when a descendant outlives its parent.
	command.WaitDelay = grace
	tree, err := prepare(command)
	if err != nil {
		return 1, fmt.Errorf("prepare child supervision: %w", err)
	}
	defer func() { err = errors.Join(err, tree.close()) }()
	interrupts := make(chan os.Signal, 2)
	signal.Notify(interrupts, os.Interrupt, terminationSignal())
	defer signal.Stop(interrupts)
	if err := command.Start(); err != nil {
		return 1, fmt.Errorf("start application: %w", err)
	}
	if err := tree.started(command); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		return 1, fmt.Errorf("attach child supervision: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	var timer *time.Timer
	var timeout <-chan time.Time
	var shutdownErr error
	stopping, forced := false, false
	interruptsSeen := 0
	cancelled := ctx.Done()
	stop := func(sig os.Signal) {
		if !stopping {
			stopping = true
			shutdownErr = errors.Join(shutdownErr, tree.signal(command.Process.Pid, sig))
			timer = time.NewTimer(grace)
			timeout = timer.C
			return
		}
		if !forced {
			forced = true
			shutdownErr = errors.Join(shutdownErr, tree.kill(command.Process.Pid))
			if timer != nil {
				timer.Stop()
			}
			timeout = nil
		}
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		select {
		case waitErr := <-done:
			cleanupErr := tree.kill(command.Process.Pid)
			code = 1
			if command.ProcessState != nil {
				code = processExitCode(command.ProcessState)
			}
			var exitError *exec.ExitError
			if errors.As(waitErr, &exitError) || errors.Is(waitErr, exec.ErrWaitDelay) {
				waitErr = nil
			}
			return code, errors.Join(waitErr, shutdownErr, cleanupErr)
		case sig := <-interrupts:
			interruptsSeen++
			if !stopping || interruptsSeen >= 2 {
				stop(sig)
			}
		case <-cancelled:
			cancelled = nil
			if !stopping {
				stop(terminationSignal())
			}
		case <-timeout:
			stop(terminationSignal())
		}
	}
}
