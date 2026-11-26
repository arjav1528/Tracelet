package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/arjav1528/Tracelet/internal/session"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/arjav1528/Tracelet/internal/config"
	"github.com/arjav1528/Tracelet/testdata/fixtures"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.yaml.in/yaml/v3"
	"google.golang.org/protobuf/proto"
)

func TestRunCommandCapturesAndPreservesExit(t *testing.T) {
	runFixture(t, "exit", 7, 3)
}

func TestRunCommandKeepsReceiverForShutdownFlush(t *testing.T) {
	runFixture(t, "flush", 0, 1)
}

func runFixture(t *testing.T, role string, wantCode, wantSpans int) {
	t.Helper()
	root := t.TempDir()
	configPath, err := config.Init(root, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Storage.Path = filepath.Join(root, "data", "traces.db")
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(root, "ready")
	t.Setenv("TRACELET_CLI_FIXTURE", role)
	t.Setenv("TRACELET_CLI_READY", ready)
	for _, key := range []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_PROTOCOL", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL"} {
		t.Setenv(key, "")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	childCtx, stopChild := context.WithCancel(ctx)
	defer stopChild()
	command := NewRoot("test", "test")
	command.SetContext(childCtx)
	var out, stderr bytes.Buffer
	command.SetOut(&out)
	command.SetErr(&stderr)
	command.SetArgs([]string{"run", "--config", configPath, "--", os.Args[0], "-test.run=^TestRunChildFixture$"})
	finished := make(chan error, 1)
	go func() { finished <- command.Execute() }()
	if role == "flush" {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		waiting := true
		for waiting {
			select {
			case err := <-finished:
				t.Fatalf("child ended before readiness: %v\n%s", err, stderr.String())
			case <-ctx.Done():
				t.Fatal("child did not become ready")
			case <-ticker.C:
				if _, err := os.Stat(ready); err == nil {
					waiting = false
				}
			}
		}
		stopChild()
	}
	select {
	case err = <-finished:
	case <-ctx.Done():
		t.Fatal("supervision did not finish")
	}
	if wantCode == 0 {
		if err != nil {
			t.Fatalf("run: %v\n%s", err, stderr.String())
		}
	} else {
		var exit *ExitError
		if !errors.As(err, &exit) || exit.Code != wantCode {
			t.Fatalf("exit=%v want%d\n%s", err, wantCode, stderr.String())
		}
	}
	db, err := sql.Open("sqlite", cfg.Storage.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow("SELECT count(*) FROM spans").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != wantSpans {
		t.Fatalf("stored%d spans want%d", count, wantSpans)
	}
}

func TestRunChildFixture(t *testing.T) {
	role := os.Getenv("TRACELET_CLI_FIXTURE")
	if role == "" {
		return
	}
	if role == "exit" {
		payload, err := proto.Marshal(fixtures.Request("simple"))
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.Post(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"), "application/x-protobuf", bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("export status%d", response.StatusCode)
		}
		fmt.Println("child output")
		os.Exit(7)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	exporter, err := otlptracehttp.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(time.Hour)))
	_, span := provider.Tracer("cli.fixture").Start(context.Background(), "final-shutdown-span")
	span.End()
	if err := os.WriteFile(os.Getenv("TRACELET_CLI_READY"), []byte("ready"), 0600); err != nil {
		t.Fatal(err)
	}
	<-ctx.Done()
	flush, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := provider.Shutdown(flush); err != nil {
		t.Fatal(err)
	}
}

func TestRunStartupFailureReleasesOwnership(t *testing.T) {
	root := t.TempDir()
	path, err := config.Init(root, false)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Storage.Path = filepath.Join(root, "traces.db")
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	cfg.Collector.Port = listener.Addr().(*net.TCPAddr).Port
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	command := NewRoot("test", "test")
	command.SetArgs([]string{"run", "--config", path, "--", "must-not-start"})
	if err = command.Execute(); err == nil {
		t.Fatal("occupied receiver should fail")
	}
	lock, err := session.Acquire(cfg.Storage.Path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
}
