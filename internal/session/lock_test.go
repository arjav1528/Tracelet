package session

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestExclusiveOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "traces.db.lock")
	lock, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if other, err := Acquire(path); !errors.Is(err, ErrOwned) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("expected owned error, got %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
}

func TestOwnershipReleasedAfterCrash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traces.db.lock")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLockChild$")
	child.Env = append(os.Environ(), "TRACELET_TEST_LOCK="+path)
	output, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	input, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = child.Process.Kill() }()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "locked\n" {
		t.Fatalf("child readiness: %q %v", line, err)
	}
	if other, err := Acquire(path); !errors.Is(err, ErrOwned) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("concurrent owner: %v", err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	lock, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
}

func TestLockChild(t *testing.T) {
	path := os.Getenv("TRACELET_TEST_LOCK")
	if path == "" {
		return
	}
	lock, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	fmt.Println("locked")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}
