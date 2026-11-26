package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionOutput(t *testing.T) {
	command := NewRoot("v0.1.0-test", "abc123")
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"version"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := "Tracelet v0.1.0-test\ncommit: abc123\nprotocol: otlp-http-protobuf\n"
	if output.String() != want {
		t.Fatalf("got %q, want %q", output.String(), want)
	}
}

func TestUnknownCommand(t *testing.T) {
	command := NewRoot("dev", "unknown")
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"not-a-command"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown-command error, got %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("unexpected command output: %q", output.String())
	}
}

func TestInitCommand(t *testing.T) {
	t.Chdir(t.TempDir())
	command := NewRoot("dev", "unknown")
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"init"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), ".tracelet/config.yaml") {
		t.Fatalf("missing config path: %q", output.String())
	}
	command = NewRoot("dev", "unknown")
	command.SetArgs([]string{"init"})
	if err := command.Execute(); err == nil {
		t.Fatal("duplicate init succeeded without --force")
	}
}
