package storage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/arjav1528/Tracelet/internal/config"
	"github.com/arjav1528/Tracelet/internal/privacy"
	"github.com/arjav1528/Tracelet/testdata/fixtures"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

func testConfig(t *testing.T) config.Config {
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
	return cfg
}

func openStore(t *testing.T, cfg config.Config) *Store {
	t.Helper()
	s, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func scalar(t *testing.T, db *sql.DB, query string) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPersistReopenRedactsAndPreserves(t *testing.T) {
	cfg := testConfig(t)
	s := openStore(t, cfg)
	request := fixtures.Request("sensitive")
	original := proto.Clone(request).(*collectortracepb.ExportTraceServiceRequest)
	expected := proto.Clone(request).(*collectortracepb.ExportTraceServiceRequest)
	if _, err := privacy.Sanitize(expected, privacy.Policy{Patterns: cfg.Privacy.RedactAttributeKeys}); err != nil {
		t.Fatal(err)
	}
	result, err := s.Ingest(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Accepted != 3 || result.Duplicates != 0 {
		t.Fatalf("result=%+v", result)
	}
	if !proto.Equal(original, request) {
		t.Fatal("Ingest changed the caller's request")
	}
	if scalar(t, s.db, "PRAGMA user_version") != 1 {
		t.Fatal("schema not migrated")
	}
	for _, want := range expected.ResourceSpans[0].ScopeSpans[0].Spans {
		var payload []byte
		var start, end int64
		if err := s.db.QueryRow("SELECT payload,start_ns,end_ns FROM spans WHERE trace_id=? AND span_id=?", hex.EncodeToString(want.TraceId), hex.EncodeToString(want.SpanId)).Scan(&payload, &start, &end); err != nil {
			t.Fatal(err)
		}
		got := new(tracepb.Span)
		if err := proto.Unmarshal(payload, got); err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(want, got) || start != int64(want.StartTimeUnixNano) || end != int64(want.EndTimeUnixNano) {
			t.Fatal("lost span fields or nanosecond precision")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(cfg.Storage.Path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(fixtures.Secret)) {
		t.Fatal("unredacted sentinel in database")
	}
	s = openStore(t, cfg)
	if scalar(t, s.db, "SELECT count(*) FROM spans") != 3 || scalar(t, s.db, "SELECT span_count FROM traces") != 3 {
		t.Fatal("committed spans did not survive restart")
	}
	var integrity string
	if err := s.db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity=%q err=%v", integrity, err)
	}
}

func TestDuplicateAndConflictingDeliveryDoNotRefreshHistory(t *testing.T) {
	s := openStore(t, testConfig(t))
	request := fixtures.Request("simple")
	if _, err := s.Ingest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE traces SET last_seen_ns=123"); err != nil {
		t.Fatal(err)
	}
	revision := scalar(t, s.db, "SELECT revision FROM metadata")
	result, err := s.Ingest(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Accepted != 0 || result.Duplicates != 3 || result.Conflicts != 0 {
		t.Fatalf("result=%+v", result)
	}
	request.ResourceSpans[0].ScopeSpans[0].Spans[1].Name = "conflicting operation"
	result, err = s.Ingest(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Accepted != 0 || result.Duplicates != 3 || result.Conflicts != 1 {
		t.Fatalf("conflict result=%+v", result)
	}
	if scalar(t, s.db, "SELECT last_seen_ns FROM traces") != 123 || scalar(t, s.db, "SELECT revision FROM metadata") != revision || scalar(t, s.db, "SELECT span_count FROM traces") != 3 {
		t.Fatal("retry changed history/revision/count")
	}
	var name string
	if err := s.db.QueryRow("SELECT name FROM spans WHERE span_id='0000000000000002'").Scan(&name); err != nil || name == "conflicting operation" {
		t.Fatal("first committed span did not win")
	}
}

func TestLateSpansUpdateSummaryAndCanonicalMetadata(t *testing.T) {
	s := openStore(t, testConfig(t))
	request := fixtures.Request("partial")
	extra := &commonpb.KeyValue{Key: "synthetic.metadata", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{BoolValue: true}}}
	request.ResourceSpans[0].Resource.Attributes = append(request.ResourceSpans[0].Resource.Attributes, extra)
	if _, err := s.Ingest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE traces SET last_seen_ns=123"); err != nil {
		t.Fatal(err)
	}
	request = fixtures.Request("simple")
	request.ResourceSpans[0].Resource.Attributes = append(request.ResourceSpans[0].Resource.Attributes, extra)
	slices.Reverse(request.ResourceSpans[0].Resource.Attributes)
	result, err := s.Ingest(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Accepted != 2 || result.Duplicates != 1 {
		t.Fatalf("result=%+v", result)
	}
	if scalar(t, s.db, "SELECT span_count FROM traces") != 3 || scalar(t, s.db, "SELECT last_seen_ns FROM traces") <= 123 {
		t.Fatal("late spans failed to update summary")
	}
	if scalar(t, s.db, "SELECT count(*) FROM resources") != 1 || scalar(t, s.db, "SELECT count(*) FROM scopes") != 1 {
		t.Fatal("metadata duplicated")
	}
}

func TestWriteFailureRollsBackWholeBatch(t *testing.T) {
	s := openStore(t, testConfig(t))
	_, err := s.db.Exec("CREATE TRIGGER reject_second BEFORE INSERT ON spans WHEN NEW.span_id='0000000000000002' BEGIN SELECT RAISE(ABORT,'synthetic write failure'); END")
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Ingest(context.Background(), fixtures.Request("simple"))
	if err == nil || result.Accepted != 0 {
		t.Fatalf("expected rollback, got %+v %v", result, err)
	}
	for _, table := range []string{"spans", "traces", "resources", "scopes"} {
		if scalar(t, s.db, "SELECT count(*) FROM "+table) != 0 {
			t.Fatalf("partial transaction in %s", table)
		}
	}
	if scalar(t, s.db, "SELECT revision FROM metadata") != 0 {
		t.Fatal("failed transaction advanced revision")
	}
}

func TestCancelledAndInvalidBatchesWriteNothing(t *testing.T) {
	s := openStore(t, testConfig(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Ingest(ctx, fixtures.Request("simple")); err == nil {
		t.Fatal("accepted canceled write")
	}
	request := fixtures.Request("simple")
	request.ResourceSpans[0].ScopeSpans[0].Spans[1].SpanId = nil
	if _, err := s.Ingest(context.Background(), request); err == nil {
		t.Fatal("accepted invalid span")
	}
	if scalar(t, s.db, "SELECT count(*) FROM spans") != 0 {
		t.Fatal("invalid batch partially persisted")
	}
}

func TestMigrationFailureAndNewerSchemaPreserveData(t *testing.T) {
	for _, version := range []int{0, 2} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			cfg := testConfig(t)
			if err := os.MkdirAll(filepath.Dir(cfg.Storage.Path), 0700); err != nil {
				t.Fatal(err)
			}
			db, err := sql.Open("sqlite", cfg.Storage.Path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("CREATE TABLE spans (kept TEXT); INSERT INTO spans VALUES ('preserve me'); PRAGMA user_version=" + string(rune('0'+version))); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			if store, err := Open(context.Background(), cfg); err == nil {
				store.Close()
				t.Fatal("accepted incompatible schema")
			}
			db, err = sql.Open("sqlite", cfg.Storage.Path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var kept string
			if err := db.QueryRow("SELECT kept FROM spans").Scan(&kept); err != nil || kept != "preserve me" {
				t.Fatal("migration destroyed old data")
			}
			if scalar(t, db, "PRAGMA user_version") != int64(version) {
				t.Fatal("failed migration changed version")
			}
			if scalar(t, db, "SELECT count(*) FROM sqlite_master WHERE name='metadata'") != 0 {
				t.Fatal("failed migration left partial tables")
			}
		})
	}
}

func TestProjectMismatchRefusesReuse(t *testing.T) {
	cfg := testConfig(t)
	s := openStore(t, cfg)
	if _, err := s.Ingest(context.Background(), fixtures.Request("simple")); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	cfg.Project.ID = strings.Repeat("f", 32)
	if s, err := Open(context.Background(), cfg); err == nil {
		s.Close()
		t.Fatal("mixed project databases")
	}
}

func TestPrivateFilesAndUnusablePath(t *testing.T) {
	cfg := testConfig(t)
	s := openStore(t, cfg)
	if runtime.GOOS != "windows" {
		for _, path := range []string{cfg.Storage.Path, cfg.Storage.Path + "-wal", cfg.Storage.Path + "-shm"} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm()&0077 != 0 {
				t.Fatalf("non-private file %s: %o", path, info.Mode().Perm())
			}
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	cfg.Storage.Path = filepath.Dir(cfg.Storage.Path)
	if s, err := Open(context.Background(), cfg); err == nil {
		s.Close()
		t.Fatal("accepted directory as database path")
	}
}

func TestDiskFullRejectsWithoutPartialData(t *testing.T) {
	s := openStore(t, testConfig(t))
	pages := scalar(t, s.db, "PRAGMA page_count")
	if _, err := s.db.Exec("PRAGMA max_page_count=" + strconv.FormatInt(pages, 10)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ingest(context.Background(), fixtures.Request("large")); err == nil {
		t.Fatal("write succeeded beyond SQLite disk budget")
	}
	if scalar(t, s.db, "SELECT count(*) FROM spans") != 0 {
		t.Fatal("disk full left partial batch")
	}
	var integrity string
	if err := s.db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatal("disk full corrupted database")
	}
}

func TestConfiguredBoundsAndEmptyBatch(t *testing.T) {
	cfg := testConfig(t)
	cfg.Limits.MaxSpansPerBatch = 2
	s := openStore(t, cfg)
	if _, err := s.Ingest(context.Background(), fixtures.Request("simple")); err == nil {
		t.Fatal("ignored configured span bound")
	}
	if result, err := s.Ingest(context.Background(), &collectortracepb.ExportTraceServiceRequest{}); err != nil || result.Accepted != 0 {
		t.Fatalf("empty batch: %+v %v", result, err)
	}
	if scalar(t, s.db, "SELECT count(*) FROM spans") != 0 || scalar(t, s.db, "SELECT revision FROM metadata") != 0 {
		t.Fatal("empty/rejected batches changed database")
	}
}

func TestReadOnlyWriteFailure(t *testing.T) {
	s := openStore(t, testConfig(t))
	if _, err := s.db.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Ingest(context.Background(), fixtures.Request("simple")); err == nil {
		t.Fatal("read-only write reported success")
	}
	if scalar(t, s.db, "SELECT count(*) FROM spans") != 0 {
		t.Fatal("read-only write changed data")
	}
}

func TestMetadataSchemaIdentityIsNotDeduplicatedAway(t *testing.T) {
	s := openStore(t, testConfig(t))
	request := fixtures.Request("partial")
	if _, err := s.Ingest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request = fixtures.Request("late-parent")
	request.ResourceSpans[0].SchemaUrl = "https://example.invalid/resource/2"
	request.ResourceSpans[0].ScopeSpans[0].SchemaUrl = "https://example.invalid/scope/2"
	if _, err := s.Ingest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if scalar(t, s.db, "SELECT count(*) FROM resources") != 2 || scalar(t, s.db, "SELECT count(*) FROM scopes") != 2 {
		t.Fatal("schema URLs disappeared from metadata identity")
	}
}

func TestOpenRefusesUnrecognizedSchemaZeroWithoutChangingBytes(t *testing.T) {
	cfg := testConfig(t)
	if err := os.MkdirAll(filepath.Dir(cfg.Storage.Path), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", cfg.Storage.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE unrelated (value TEXT); INSERT INTO unrelated VALUES ('keep unrelated history')"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(cfg.Storage.Path)
	if err != nil {
		t.Fatal(err)
	}
	if s, err := Open(context.Background(), cfg); err == nil {
		s.Close()
		t.Fatal("adopted unrelated unversioned database")
	}
	after, err := os.ReadFile(cfg.Storage.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("refusal changed the original database bytes")
	}
}

func TestOpenRefusesIncompleteClaimedSchema(t *testing.T) {
	for _, corruption := range []string{"DROP TABLE scopes", "ALTER TABLE spans DROP COLUMN payload"} {
		t.Run(corruption, func(t *testing.T) {
			cfg := testConfig(t)
			s := openStore(t, cfg)
			if _, err := s.db.Exec(corruption); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			if reopened, err := Open(context.Background(), cfg); err == nil {
				reopened.Close()
				t.Fatal("opened incomplete claimed schema1")
			}
		})
	}
}

func TestMigrationFailureRollsBackDDL(t *testing.T) {
	cfg := testConfig(t)
	if err := os.MkdirAll(filepath.Dir(cfg.Storage.Path), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", cfg.Storage.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE scopes (preserved TEXT); INSERT INTO scopes VALUES ('original')"); err != nil {
		t.Fatal(err)
	}
	s := &Store{db: db, cfg: cfg}
	if err := s.migrate(context.Background()); err == nil {
		t.Fatal("migration ignored conflicting schema")
	}
	if scalar(t, db, "PRAGMA user_version") != 0 || scalar(t, db, "SELECT count(*) FROM sqlite_master WHERE name IN ('metadata','resources')") != 0 {
		t.Fatal("migration left partial DDL")
	}
	var value string
	if err := db.QueryRow("SELECT preserved FROM scopes").Scan(&value); err != nil || value != "original" {
		t.Fatal("migration damaged original data")
	}
}

func TestNewDatabaseSupportsIncrementalReclamation(t *testing.T) {
	s := openStore(t, testConfig(t))
	if mode := scalar(t, s.db, "PRAGMA auto_vacuum"); mode != 2 {
		t.Fatalf("auto_vacuum=%d want incremental (2)", mode)
	}
}

func TestDatabaseURIKeepsPathsOutOfAuthorityAndQuery(t *testing.T) {
	for _, test := range []struct{ path, wantPath string }{
		{"/tmp/traces ?#.db", "/tmp/traces ?#.db"},
		{"C:/Users/example/traces.db", "/C:/Users/example/traces.db"},
		{`C:\Users\example\traces.db`, "/C:/Users/example/traces.db"},
	} {
		uri := databaseURI(test.path)
		parsed, err := url.Parse(uri.String())
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Host != "" || parsed.Path != test.wantPath || parsed.RawQuery != "" || parsed.Fragment != "" {
			t.Fatalf("unsafe SQLite URI for %q: %s", test.path, uri.String())
		}
	}
}

func TestExistingEmptySchemaZeroCanEnableReclamation(t *testing.T) {
	cfg := testConfig(t)
	if err := os.MkdirAll(filepath.Dir(cfg.Storage.Path), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", cfg.Storage.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE removed (value TEXT); DROP TABLE removed"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s := openStore(t, cfg)
	if mode := scalar(t, s.db, "PRAGMA auto_vacuum"); mode != 2 {
		t.Fatalf("existing empty DB mode=%d", mode)
	}
	if scalar(t, s.db, "PRAGMA user_version") != SchemaVersion {
		t.Fatal("empty database did not migrate")
	}
}

func TestOpenRefusesSchemaWithoutIncrementalReclamation(t *testing.T) {
	cfg := testConfig(t)
	s := openStore(t, cfg)
	if _, err := s.Ingest(context.Background(), fixtures.Request("simple")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("PRAGMA auto_vacuum=NONE; VACUUM"); err != nil {
		t.Fatal(err)
	}
	if mode := scalar(t, s.db, "PRAGMA auto_vacuum"); mode != 0 {
		t.Fatalf("test did not disable reclamation: %d", mode)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(cfg.Storage.Path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened, err := Open(context.Background(), cfg); err == nil {
		reopened.Close()
		t.Fatal("silently accepted incompatible reclamation mode")
	}
	after, err := os.ReadFile(cfg.Storage.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("refused schema was modified")
	}
}
