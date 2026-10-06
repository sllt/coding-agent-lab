// Package sqlite is the only writer of Lab metadata. PRAGMA settings are
// applied to every new connection; a single write lock keeps transactions short.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sllt/agentlab/migrations"
)

// Store owns one SQLite database.
type Store struct {
	db      *sql.DB
	writeMu sync.Mutex
	path    string
}

// Open creates the database file, migrates it, and checks connection pragmas.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	s := &Store{db: db, path: path}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := assertPragmas(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Path() string { return s.path }

// Migrate applies embedded SQL files in name order. Each file is one transaction.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM schema_migrations WHERE version=?`, name).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			return err
		}
		if err := s.WithTx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return fmt.Errorf("migration %s: %w", name, err)
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)`, name, time.Now().UTC().Format(time.RFC3339Nano))
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

// WithTx runs a short write transaction. The callback must not start goroutines
// that use the transaction.
func (s *Store) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ErrConflict is returned when a compare-and-swap misses.
var ErrConflict = errors.New("cas conflict")

// ErrAdminExists is returned when a second administrator setup is attempted.
var ErrAdminExists = errors.New("admin exists")

// CASUpdate updates one row when row_version still matches and increments it.
func CASUpdate(ctx context.Context, tx *sql.Tx, query string, args ...any) error {
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrConflict
	}
	return nil
}

func assertPragmas(ctx context.Context, db *sql.DB) error {
	for i := 0; i < 2; i++ {
		conn, err := db.Conn(ctx)
		if err != nil {
			return err
		}
		if err := checkConn(ctx, conn); err != nil {
			_ = conn.Close()
			return err
		}
		_ = conn.Close()
	}
	return nil
}

func checkConn(ctx context.Context, conn *sql.Conn) error {
	var fk int
	if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil {
		return err
	}
	if fk != 1 {
		return fmt.Errorf("foreign_keys=%d, want 1", fk)
	}
	var sync string
	if err := conn.QueryRowContext(ctx, `PRAGMA synchronous`).Scan(&sync); err != nil {
		return err
	}
	// modernc returns the integer mode. FULL is 2.
	if sync != "2" && !strings.EqualFold(sync, "FULL") {
		return fmt.Errorf("synchronous=%s, want FULL", sync)
	}
	var mode string
	if err := conn.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil {
		return err
	}
	if !strings.EqualFold(mode, "wal") {
		return fmt.Errorf("journal_mode=%s, want wal", mode)
	}
	var busy int
	if err := conn.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busy); err != nil {
		return err
	}
	if busy < 5000 {
		return fmt.Errorf("busy_timeout=%d, want >= 5000", busy)
	}
	return nil
}

// DB exposes the handle for repository methods in this package only.
func (s *Store) DB() *sql.DB { return s.db }
