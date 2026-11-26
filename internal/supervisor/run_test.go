package supervisor

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"tracelet.local/tracelet/internal/session"
)

func helper(mode string) ([]string, []string) {
	return []string{os.Args[0], "-test.run=^TestHelperProcess$"}, append(os.Environ(), "TRACELET_SUPERVISOR_HELPER="+mode)
}

func TestRunPreservesExitArgumentsAndStreams(t *testing.T) {
	for _, mode := range []string{"exit0", "exit7"} {
		t.Run(mode, func(t *testing.T) {
			args, env := helper(mode)
			args = append(args, "--", "argument with spaces", "$literal")
			var stdout, stderr bytes.Buffer
			code, err := Run(context.Background(), args, env, &stdout, &stderr, time.Second)
			want := 0
			if mode == "exit7" {
				want = 7
			}
			if err != nil || code != want {
				t.Fatalf("code=%d err=%v", code, err)
			}
			if !strings.Contains(stdout.String(), "argument with spaces|$literal") || stderr.String() != "child stderr\n" {
				t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunLaunchFailure(t *testing.T) {
	code, err := Run(context.Background(), []string{filepath.Join(t.TempDir(), "missing")}, nil, nil, nil, time.Second)
	if err == nil || code == 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestContextCancellationAllowsFinalOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("graceful console signal requires a shared Windows console; native runner gate")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	args, env := helper("flush")
	output := newReadyWriter("ready\n")
	var stderr bytes.Buffer
	done := make(chan error, 1)
	go func() {
		code, err := Run(ctx, args, env, output, &stderr, 2*time.Second)
		if code != 0 && err == nil {
			err = fmt.Errorf("code=%d", code)
		}
		done <- err
	}()
	select {
	case <-output.ready:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("child did not become ready")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown hung")
	}
	if !strings.Contains(output.String(), "flushed\n") {
		t.Fatalf("final output lost: %q", output.String())
	}
}

func TestGraceExpiryForcesTermination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows signal/exit mapping needs a native console runner")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	args, env := helper("ignore")
	output := newReadyWriter("ready\n")
	done := make(chan error, 1)
	go func() {
		code, err := Run(ctx, args, env, output, output, 50*time.Millisecond)
		if err == nil && code != 137 {
			err = fmt.Errorf("code=%d want 137", code)
		}
		done <- err
	}()
	select {
	case <-output.ready:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("child not ready")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("grace deadline did not terminate child")
	}
}

func TestRunCleansGrandchildrenOnNormalExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grandchild.lock")
	args, env := helper("spawn")
	env = append(env, "TRACELET_GRANDCHILD_LOCK="+path)
	var output bytes.Buffer
	code, err := Run(context.Background(), args, env, &output, &output, time.Second)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v output=%s", code, err, output.String())
	}
	if !strings.Contains(output.String(), "grandchild ready") {
		t.Fatalf("grandchild not started: %s", output.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		lock, err := session.Acquire(path)
		if err == nil {
			lock.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("grandchild retained project lock after parent exit")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSecondInterruptForcesShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Process.Signal interrupt is unsupported on Windows")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args, env := helper("supervisor")
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.Env = env
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	reader := bufio.NewReader(stdout)
	if line, err := reader.ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("readiness %q %v", line, err)
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "signal received\n" {
		t.Fatalf("first signal %q %v", line, err)
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "forced:137:<nil>\n" {
		t.Fatalf("forced result %q %v stderr=%s", line, err, stderr.String())
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("helper: %v %s", err, stderr.String())
	}
}

func TestSignalContextDoesNotSkipGrace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Process.Signal interrupt is unsupported on Windows")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	args, env := helper("supervisor-context")
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.Env = env
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer command.Process.Kill()
	reader := bufio.NewReader(stdout)
	if line, err := reader.ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("readiness %q %v", line, err)
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "flushed\n" {
		t.Fatalf("first interrupt skipped flush: %q %v", line, err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "result:0:<nil>\n" {
		t.Fatalf("result %q %v", line, err)
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestHelperProcess(t *testing.T) {
	mode := os.Getenv("TRACELET_SUPERVISOR_HELPER")
	if mode == "" {
		return
	}
	switch mode {
	case "exit0", "exit7":
		for i, arg := range os.Args {
			if arg == "--" {
				fmt.Println(strings.Join(os.Args[i+1:], "|"))
				break
			}
		}
		fmt.Fprintln(os.Stderr, "child stderr")
		if mode == "exit7" {
			os.Exit(7)
		}
		os.Exit(0)
	case "flush", "flush-delayed", "ignore":
		ch := make(chan os.Signal, 2)
		signal.Notify(ch, os.Interrupt, terminationSignal())
		fmt.Println("ready")
		for range ch {
			if mode == "flush" || mode == "flush-delayed" {
				if mode == "flush-delayed" {
					time.Sleep(100 * time.Millisecond)
				}
				fmt.Println("flushed")
				os.Exit(0)
			}
			fmt.Println("signal received")
		}
	case "grandchild":
		lock, err := session.Acquire(os.Getenv("TRACELET_GRANDCHILD_LOCK"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(3)
		}
		defer lock.Close()
		fmt.Println("held")
		for {
			time.Sleep(time.Hour)
		}
	case "spawn":
		args, env := helper("grandchild")
		child := exec.Command(args[0], args[1:]...)
		child.Env = env
		stdout, err := child.StdoutPipe()
		if err != nil {
			os.Exit(4)
		}
		if err := child.Start(); err != nil {
			os.Exit(5)
		}
		if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "held\n" {
			os.Exit(6)
		}
		fmt.Println("grandchild ready")
		os.Exit(0)
	case "supervisor-context":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		args, env := helper("flush-delayed")
		code, err := Run(ctx, args, env, os.Stdout, os.Stderr, 2*time.Second)
		fmt.Printf("result:%d:%v\n", code, err)
		os.Exit(0)
	case "supervisor":
		args, env := helper("ignore")
		code, err := Run(context.Background(), args, env, os.Stdout, os.Stderr, 30*time.Second)
		fmt.Printf("forced:%d:%v\n", code, err)
		os.Exit(0)
	}
	os.Exit(9)
}

type readyWriter struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	ready  chan struct{}
	marker string
	once   sync.Once
}

func newReadyWriter(marker string) *readyWriter {
	return &readyWriter{ready: make(chan struct{}), marker: marker}
}
func (w *readyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buffer.Write(p)
	if strings.Contains(w.buffer.String(), w.marker) {
		w.once.Do(func() { close(w.ready) })
	}
	return n, err
}
func (w *readyWriter) String() string { w.mu.Lock(); defer w.mu.Unlock(); return w.buffer.String() }
