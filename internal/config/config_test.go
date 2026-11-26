package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInitAndLoad(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	path, err := Init(root, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != 1 || len(cfg.Project.ID) != 32 || cfg.Collector.Bind != "127.0.0.1" || cfg.Collector.Port != 0 || cfg.Storage.RetentionDays != 7 || cfg.Storage.MaxSizeMB != 1024 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Source.WorkspaceRoot != resolvedRoot {
		t.Fatalf("workspace=%q want %q", cfg.Source.WorkspaceRoot, resolvedRoot)
	}
	if !filepath.IsAbs(cfg.Storage.Path) {
		t.Fatalf("storage path not resolved: %s", cfg.Storage.Path)
	}
	if cfg.Limits.MaxWireBytes != 4<<20 || cfg.Limits.MaxDecodedBytes != 16<<20 || cfg.ShutdownGraceSeconds != 5 {
		t.Fatalf("unexpected bounds: %+v", cfg)
	}
	if _, err := Init(root, false); err == nil {
		t.Fatal("overwrote existing config")
	}
	again, err := Load(path)
	if err != nil || again.Project.ID != cfg.Project.ID {
		t.Fatal("init changed existing project identity")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm() != 0600 {
			t.Fatalf("permissions %o", info.Mode().Perm())
		}
	}
}

func TestLoadRejectsUnsafeOrMalformedConfig(t *testing.T) {
	for name, body := range map[string]string{
		"nul-path": "storage:\n  path: \"\\0\"",
		"yaml":     "project: [", "unknown": "unexpected: true", "version": "version: 2", "invalid-id": "project:\n  id: ../../outside",
		"remote": "collector:\n  bind: 0.0.0.0", "ui-remote": "ui:\n  bind: 192.168.1.2", "negative-port": "collector:\n  port: -1", "large-port": "ui:\n  port: 65536",
		"body": "privacy:\n  capture_http_bodies: true", "zero-budget": "storage:\n  max_size_mb: 0", "negative-age": "storage:\n  retention_days: -1", "zero-wire": "limits:\n  max_wire_bytes: 0", "multi-doc": "---\nversion: 1\n---\nversion: 1", "duplicate-key": "version: 1\nversion: 1",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path, err := Init(root, false)
			if err != nil {
				t.Fatal(err)
			}
			// Prefix a minimal valid identity, then replace the selected top-level configuration section.
			content := "version: 1\nproject:\n  id: 11111111111111111111111111111111\n  name: sample\n" + body + "\n"
			if strings.HasPrefix(body, "version:") || strings.HasPrefix(body, "project:") || name == "multi-doc" || name == "duplicate-key" {
				content = body + "\n"
			}
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatalf("accepted %s", body)
			}
		})
	}
}

func TestLoadResolvesPathsAndAddsPrivacyDefaults(t *testing.T) {
	root := t.TempDir()
	path, err := Init(root, false)
	if err != nil {
		t.Fatal(err)
	}
	content := "version: 1\nproject:\n  id: 11111111111111111111111111111111\n  name: sample\nstorage:\n  path: local/data.db\nsource:\n  workspace_root: .\nprivacy:\n  redact_attribute_keys: [custom.private]\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Storage.Path != filepath.Join(root, "local/data.db") {
		t.Fatalf("path=%s", cfg.Storage.Path)
	}
	if len(cfg.Privacy.RedactAttributeKeys) < 2 {
		t.Fatal("custom pattern disabled privacy defaults")
	}
}

func TestInitRefusesSymlinkDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges vary on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, ".tracelet")); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(root, false); err == nil {
		t.Fatal("followed symlink during init")
	}
}

func TestInitForceKeepsExistingDataAndSource(t *testing.T) {
	root := t.TempDir()
	path, err := Init(root, false)
	if err != nil {
		t.Fatal(err)
	}
	before, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "main.go")
	if err := os.WriteFile(source, []byte("existing source"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(root, true); err != nil {
		t.Fatal(err)
	}
	after, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if before.Project.ID == after.Project.ID {
		t.Fatal("force must create the documented new project identity")
	}
	content, err := os.ReadFile(source)
	if err != nil || string(content) != "existing source" {
		t.Fatal("modified application source")
	}
}

func TestLoadPermissionFailure(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("POSIX non-root permission check")
	}
	path, err := Init(t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	if _, err := Load(path); err == nil {
		t.Fatal("read inaccessible configuration")
	}
}
