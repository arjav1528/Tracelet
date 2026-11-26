package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
	"tracelet.local/tracelet/internal/config"
)

const SchemaVersion = 1

//go:embed migrations/001_init.sql
var initialSchema string

type Store struct {
	db     *sql.DB
	cfg    config.Config
	writer chan struct{}
}

// Open requires the caller to hold the project ownership lock until Close returns.
func Open(ctx context.Context, cfg config.Config) (*Store, error) {
	if !filepath.IsAbs(cfg.Storage.Path) || strings.HasPrefix(cfg.Storage.Path, `\\`) || strings.HasPrefix(filepath.ToSlash(cfg.Storage.Path), "//") {
		return nil, errors.New("database path must be absolute and local")
	}
	identity, err := hex.DecodeString(cfg.Project.ID)
	if err != nil || len(identity) != 16 || cfg.Project.ID != strings.ToLower(cfg.Project.ID) {
		return nil, errors.New("invalid storage project identity")
	}
	if cfg.Limits.MaxDecodedBytes <= 0 || cfg.Limits.MaxSpansPerBatch <= 0 {
		return nil, errors.New("invalid storage ingestion limits")
	}
	if info, err := os.Lstat(cfg.Storage.Path); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("database path must be a regular file, not a symlink or directory")
		}
		if info.Size() > 0 {
			if err := checkExisting(ctx, cfg); err != nil {
				return nil, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect database path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Storage.Path), 0700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	file, err := os.OpenFile(cfg.Storage.Path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open database file: %w", err)
	}
	chmodErr := file.Chmod(0600)
	closeErr := file.Close()
	if err := errors.Join(chmodErr, closeErr); err != nil {
		return nil, fmt.Errorf("secure database file: %w", err)
	}
	uri := databaseURI(cfg.Storage.Path)
	query := uri.Query()
	for _, pragma := range []string{"auto_vacuum(INCREMENTAL)", "foreign_keys(1)", "busy_timeout(5000)", "journal_mode(WAL)", "synchronous(FULL)"} {
		query.Add("_pragma", pragma)
	}
	uri.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, fmt.Errorf("open SQLite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	s := &Store{db: db, cfg: cfg, writer: make(chan struct{}, 1)}
	if err := prepareEmptySchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func checkExisting(ctx context.Context, cfg config.Config) error {
	uri := databaseURI(cfg.Storage.Path)
	query := uri.Query()
	query.Set("mode", "ro")
	uri.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return fmt.Errorf("inspect existing database: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read database schema; original file preserved: %w", err)
	}
	if version < 0 || version > SchemaVersion {
		return fmt.Errorf("database schema %d is newer than supported schema %d; original file preserved", version, SchemaVersion)
	}
	if version == 0 {
		var objects int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'").Scan(&objects); err != nil {
			return fmt.Errorf("inspect unversioned database: %w", err)
		}
		if objects != 0 {
			return errors.New("unrecognized nonempty schema 0 database; original file preserved")
		}
	}
	if version == SchemaVersion {
		if err := validateSchema(ctx, db); err != nil {
			return err
		}
		var identity string
		if err := db.QueryRowContext(ctx, "SELECT project_id FROM metadata WHERE id=1").Scan(&identity); err != nil {
			return fmt.Errorf("read database identity; original file preserved: %w", err)
		}
		if identity != cfg.Project.ID {
			return errors.New("database belongs to another project; choose its original configuration or a different storage path")
		}
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start migration: %w", err)
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version < 0 || version > SchemaVersion {
		return fmt.Errorf("database schema %d is newer than supported schema %d", version, SchemaVersion)
	}
	if version == 0 {
		if _, err := tx.ExecContext(ctx, initialSchema); err != nil {
			return fmt.Errorf("migrate schema; original data preserved: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO metadata(id,project_id,revision) VALUES(1,?,0)", s.cfg.Project.ID); err != nil {
			return fmt.Errorf("initialize database identity: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
			return fmt.Errorf("set schema version: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migration: %w", err)
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func databaseURI(path string) url.URL {
	path = filepath.ToSlash(path)
	if len(path) >= 2 && path[1] == ':' {
		path = "/" + strings.ReplaceAll(path, `\`, "/")
	}
	return url.URL{Scheme: "file", Path: path}
}

func validateSchema(ctx context.Context, db *sql.DB) error {
	for _, query := range []string{
		"SELECT id,project_id,revision FROM metadata LIMIT 0",
		"SELECT resource_id,payload FROM resources LIMIT 0",
		"SELECT scope_id,payload FROM scopes LIMIT 0",
		"SELECT trace_id,start_ns,end_ns,span_count,error_count,first_seen_ns,last_seen_ns,revision FROM traces LIMIT 0",
		"SELECT trace_id,span_id,parent_span_id,name,service_name,start_ns,end_ns,kind,status,resource_id,scope_id,payload FROM spans LIMIT 0",
	} {
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return fmt.Errorf("incomplete schema 1; original file preserved: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("inspect schema: %w", err)
		}
	}
	var mode int
	if err := db.QueryRowContext(ctx, "PRAGMA auto_vacuum").Scan(&mode); err != nil {
		return fmt.Errorf("inspect reclamation mode: %w", err)
	}
	if mode != 2 {
		return errors.New("schema 1 lacks incremental reclamation; original file preserved; use a fresh storage path or a separately reviewed migration")
	}
	return nil
}

// Only a proven empty schema 0 may need a one-time rebuild to enable reclamation.
func prepareEmptySchema(ctx context.Context, db *sql.DB) error {
	var version, objects, mode int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version != 0 {
		return nil
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_schema WHERE name NOT LIKE 'sqlite_%'").Scan(&objects); err != nil {
		return fmt.Errorf("inspect empty database: %w", err)
	}
	if objects != 0 {
		return errors.New("refusing to prepare a nonempty unversioned database")
	}
	if err := db.QueryRowContext(ctx, "PRAGMA auto_vacuum").Scan(&mode); err != nil {
		return fmt.Errorf("read reclamation mode: %w", err)
	}
	if mode == 2 {
		return nil
	}
	if _, err := db.ExecContext(ctx, "PRAGMA auto_vacuum=INCREMENTAL; VACUUM"); err != nil {
		return fmt.Errorf("prepare empty database reclamation; choose a fresh storage path: %w", err)
	}
	if err := db.QueryRowContext(ctx, "PRAGMA auto_vacuum").Scan(&mode); err != nil || mode != 2 {
		return errors.New("cannot enable incremental reclamation safely; choose a fresh storage path")
	}
	return nil
}
