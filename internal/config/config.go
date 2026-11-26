package config

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	Version int `yaml:"version"`
	Project struct {
		ID   string `yaml:"id"`
		Name string `yaml:"name"`
	} `yaml:"project"`
	Collector struct {
		Bind     string `yaml:"bind"`
		Port     int    `yaml:"port"`
		Protocol string `yaml:"protocol"`
	} `yaml:"collector"`
	UI struct {
		Bind        string `yaml:"bind"`
		Port        int    `yaml:"port"`
		OpenBrowser bool   `yaml:"open_browser"`
	} `yaml:"ui"`
	Storage struct {
		Path          string `yaml:"path"`
		RetentionDays int    `yaml:"retention_days"`
		MaxSizeMB     int64  `yaml:"max_size_mb"`
	} `yaml:"storage"`
	Privacy struct {
		CaptureHTTPBodies   bool     `yaml:"capture_http_bodies"`
		RedactAttributeKeys []string `yaml:"redact_attribute_keys"`
	} `yaml:"privacy"`
	Source struct {
		WorkspaceRoot string `yaml:"workspace_root"`
	} `yaml:"source"`
	ShutdownGraceSeconds int    `yaml:"shutdown_grace_seconds"`
	Limits               Limits `yaml:"limits"`
	Path                 string `yaml:"-"`
}

type Limits struct {
	MaxWireBytes             int64 `yaml:"max_wire_bytes"`
	MaxDecodedBytes          int64 `yaml:"max_decoded_bytes"`
	MaxSpansPerBatch         int   `yaml:"max_spans_per_batch"`
	MaxConcurrentRequests    int   `yaml:"max_concurrent_requests"`
	MaxAttributes            int   `yaml:"max_attributes"`
	MaxKeyBytes              int   `yaml:"max_key_bytes"`
	MaxValueBytes            int   `yaml:"max_value_bytes"`
	MaxDepth                 int   `yaml:"max_depth"`
	MaxEvents                int   `yaml:"max_events"`
	MaxLinks                 int   `yaml:"max_links"`
	MaxPendingBatches        int   `yaml:"max_pending_batches"`
	MaxPendingBytes          int64 `yaml:"max_pending_bytes"`
	ReadHeaderTimeoutSeconds int   `yaml:"read_header_timeout_seconds"`
	ReadTimeoutSeconds       int   `yaml:"read_timeout_seconds"`
	WriteTimeoutSeconds      int   `yaml:"write_timeout_seconds"`
	IdleTimeoutSeconds       int   `yaml:"idle_timeout_seconds"`
}

func Load(path string) (Config, error) {
	c := defaults()
	c.Version = 0 // Persisted configuration must declare its schema version.
	abs, err := filepath.Abs(path)
	if err != nil {
		return c, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return c, fmt.Errorf("open project config: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return c, fmt.Errorf("read project config: %w", err)
	}
	if len(data) > 1<<20 {
		return c, errors.New("project config exceeds 1 MiB")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&c); err != nil {
		return c, fmt.Errorf("decode project config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return c, errors.New("project config must contain exactly one YAML document")
	}
	if err := c.validate(); err != nil {
		return c, err
	}
	c.Path = abs
	root := filepath.Dir(abs)
	if filepath.Base(root) == ".tracelet" {
		root = filepath.Dir(root)
	}
	if !filepath.IsAbs(c.Source.WorkspaceRoot) {
		c.Source.WorkspaceRoot = filepath.Join(root, c.Source.WorkspaceRoot)
	}
	c.Source.WorkspaceRoot, err = filepath.EvalSymlinks(c.Source.WorkspaceRoot)
	if err != nil {
		return c, fmt.Errorf("resolve source workspace: %w", err)
	}
	info, err := os.Stat(c.Source.WorkspaceRoot)
	if err != nil || !info.IsDir() {
		return c, errors.New("source workspace must be an existing directory")
	}
	if c.Storage.Path == "" {
		base, err := dataDirectory()
		if err != nil {
			return c, err
		}
		c.Storage.Path = filepath.Join(base, "tracelet", "projects", c.Project.ID, "traces.db")
	} else if !filepath.IsAbs(c.Storage.Path) {
		c.Storage.Path = filepath.Join(root, c.Storage.Path)
	}
	for _, pattern := range sensitiveKeys {
		found := false
		for _, existing := range c.Privacy.RedactAttributeKeys {
			if strings.EqualFold(pattern, existing) {
				found = true
				break
			}
		}
		if !found {
			c.Privacy.RedactAttributeKeys = append(c.Privacy.RedactAttributeKeys, pattern)
		}
	}
	return c, nil
}

func (c Config) validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported config version %d; expected 1", c.Version)
	}
	id, err := hex.DecodeString(c.Project.ID)
	if err != nil || len(id) != 16 || c.Project.ID != strings.ToLower(c.Project.ID) {
		return errors.New("project.id must be 32 lowercase hexadecimal characters")
	}
	if strings.TrimSpace(c.Project.Name) == "" {
		return errors.New("project.name must not be empty")
	}
	if c.Collector.Bind != "127.0.0.1" || c.UI.Bind != "127.0.0.1" {
		return errors.New("v0.1 requires collector and UI bind to 127.0.0.1")
	}
	if c.Collector.Port < 0 || c.Collector.Port > 65535 || c.UI.Port < 0 || c.UI.Port > 65535 {
		return errors.New("ports must be between 0 and 65535")
	}
	if c.Collector.Protocol != "otlp-http-protobuf" {
		return errors.New("only otlp-http-protobuf is supported")
	}
	if c.Privacy.CaptureHTTPBodies {
		return errors.New("HTTP body capture is not supported")
	}
	if c.Storage.RetentionDays <= 0 || c.Storage.RetentionDays > 36500 || c.Storage.MaxSizeMB <= 0 || c.Storage.MaxSizeMB > 1<<40 {
		return errors.New("storage retention and size must be positive and representable")
	}
	if c.ShutdownGraceSeconds <= 0 || c.ShutdownGraceSeconds > 3600 {
		return errors.New("shutdown grace must be between 1 and 3600 seconds")
	}
	if c.Source.WorkspaceRoot == "" {
		return errors.New("source.workspace_root must not be empty")
	}
	if strings.ContainsRune(c.Storage.Path, 0) || strings.ContainsRune(c.Source.WorkspaceRoot, 0) {
		return errors.New("configured paths must not contain NUL characters")
	}
	l := c.Limits
	for _, v := range []int64{l.MaxWireBytes, l.MaxDecodedBytes, l.MaxPendingBytes, int64(l.MaxSpansPerBatch), int64(l.MaxConcurrentRequests), int64(l.MaxAttributes), int64(l.MaxKeyBytes), int64(l.MaxValueBytes), int64(l.MaxDepth), int64(l.MaxEvents), int64(l.MaxLinks), int64(l.MaxPendingBatches), int64(l.ReadHeaderTimeoutSeconds), int64(l.ReadTimeoutSeconds), int64(l.WriteTimeoutSeconds), int64(l.IdleTimeoutSeconds)} {
		if v <= 0 || v > 1<<40 {
			return errors.New("all ingestion limits must be positive and representable")
		}
	}
	for _, seconds := range []int{l.ReadHeaderTimeoutSeconds, l.ReadTimeoutSeconds, l.WriteTimeoutSeconds, l.IdleTimeoutSeconds} {
		if seconds > 3600 {
			return errors.New("HTTP timeouts must not exceed 3600 seconds")
		}
	}
	if l.MaxDepth > 64 {
		return errors.New("max_depth must not exceed 64")
	}
	for _, p := range c.Privacy.RedactAttributeKeys {
		if strings.TrimSpace(p) == "" || len(p) > 256 {
			return errors.New("redaction patterns must contain 1–256 bytes")
		}
	}
	return nil
}

func Init(root string, overwrite bool) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(abs, ".tracelet")
	if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("refusing to initialize through a symlinked .tracelet directory")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}
	c := defaults()
	c.Project.Name = filepath.Base(abs)
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	c.Project.ID = hex.EncodeToString(id)
	data, err := yaml.Marshal(c)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "config.yaml")
	if !overwrite {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return "", fmt.Errorf("create config (use init --force only to replace it): %w", err)
		}
		_, writeErr := f.Write(data)
		closeErr := f.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			_ = os.Remove(path)
			return "", err
		}
	} else {
		f, err := os.CreateTemp(dir, ".config-*")
		if err != nil {
			return "", err
		}
		defer os.Remove(f.Name())
		_, writeErr := f.Write(data)
		closeErr := f.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return "", err
		}
		if err := os.Rename(f.Name(), path); err != nil {
			return "", fmt.Errorf("replace config: %w", err)
		}
	}
	return path, nil
}

func dataDirectory() (string, error) {
	if runtime.GOOS == "windows" {
		if p := os.Getenv("LOCALAPPDATA"); filepath.IsAbs(p) {
			return p, nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support"), nil
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(home, "AppData", "Local"), nil
	}
	if p := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(p) {
		return p, nil
	}
	return filepath.Join(home, ".local", "share"), nil
}
