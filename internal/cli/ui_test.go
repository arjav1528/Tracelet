package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arjav1528/Tracelet/internal/config"
	"github.com/arjav1528/Tracelet/internal/session"
	"go.yaml.in/yaml/v3"
)

func sessionConfig(t *testing.T) (string, config.Config) {
	t.Helper()
	root := t.TempDir()
	path, err := config.Init(root, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Storage.Path = filepath.Join(root, "data", "traces.db")
	cfg.UI.OpenBrowser = false
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path, cfg
}

func TestStatusDoesNotStartOrCreateSession(t *testing.T) {
	path, cfg := sessionConfig(t)
	command := NewRoot("test", "test")
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"status", "--config", path})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var status struct {
		Active    bool   `json:"active"`
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(output.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status.Active || status.ProjectID != cfg.Project.ID {
		t.Fatalf("bad status:%s", output.String())
	}
	if _, err := os.Stat(filepath.Dir(cfg.Storage.Path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("status created storage:%v", err)
	}
}

func TestHistoryLifecycleAndDiscovery(t *testing.T) {
	path, cfg := sessionConfig(t)
	// A crash can leave a descriptor whose PID has since been reused.
	stale, err := session.NewDescriptor(cfg.Project.ID, "history")
	if err != nil {
		t.Fatal(err)
	}
	stale.UIURL = "http://127.0.0.1:1"
	if err = session.Publish(cfg.Storage.Path+".session.json", stale); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := uiCommand(func(string) error { t.Error("--no-open invoked browser"); return nil })
	var output, diagnostics bytes.Buffer
	command.SetContext(ctx)
	command.SetOut(&output)
	command.SetErr(&diagnostics)
	command.SetArgs([]string{"--config", path, "--no-open"})
	done := make(chan error, 1)
	go func() { done <- command.Execute() }()
	var d session.Descriptor
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	waiting := true
	for waiting {
		select {
		case err := <-done:
			t.Fatalf("history ended early:%v", err)
		case <-deadline.C:
			t.Fatal("history never ready")
		case <-ticker.C:
			var err error
			d, err = session.Read(cfg.Storage.Path + ".session.json")
			if err == nil && session.Probe(ctx, d) == nil {
				waiting = false
			}
		}
	}
	if d.Role != "history" || d.ReceiverEndpoint != "" || d.InstanceID == stale.InstanceID {
		t.Fatalf("bad descriptor:%+v", d)
	}
	status := NewRoot("test", "test")
	var statusOut bytes.Buffer
	status.SetOut(&statusOut)
	status.SetArgs([]string{"status", "--config", path})
	if err := status.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(statusOut.String(), `"active":true`) || strings.Contains(statusOut.String(), d.Nonce) {
		t.Fatal("unsafe or inactive status", statusOut.String())
	}
	second := uiCommand(func(string) error { return errors.New("browser unavailable") })
	var secondOut, secondErr bytes.Buffer
	second.SetOut(&secondOut)
	second.SetErr(&secondErr)
	second.SetArgs([]string{"--config", path})
	// Enable opening only in this second invocation to test the printed fallback.
	cfg.UI.OpenBrowser = true
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = second.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(secondOut.String(), "#token="+d.Nonce) || !strings.Contains(secondErr.String(), "open the printed URL") {
		t.Fatal("missing safe browser fallback")
	}
	run := NewRoot("test", "test")
	run.SetArgs([]string{"run", "--config", path, "--", "must-not-start"})
	if err = run.Execute(); err == nil || !strings.Contains(err.Error(), "stop") {
		t.Fatalf("history conflict should explain stop:%v", err)
	}
	cancel()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if _, err = session.Read(cfg.Storage.Path + ".session.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("descriptor survived shutdown:%v", err)
	}
	lock, err := session.Acquire(cfg.Storage.Path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "#token="+d.Nonce) || !strings.Contains(diagnostics.String(), "Ctrl+C") {
		t.Fatal("missing foreground instructions")
	}
}
