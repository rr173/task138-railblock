// Package store is the SQLite persistence layer for the railway interlocking
// engine. It owns the schema, the connection and the transaction primitives;
// the service layer layers business rules on top.
//
// All write operations run inside a caller-begun IMMEDIATE transaction; Open
// sets _txlock=immediate so every BEGIN is BEGIN IMMEDIATE, serializing
// writers. A single pooled connection (SetMaxOpenConns(1)) avoids "database is
// locked" from interleaved write transactions on extra connections. The
// append-only events table is the source of truth for restart recovery:
// ReconcileAll replays it to rebuild the in-memory yard state and corrects any
// persisted row that disagrees with the replayed state.
package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // pure-Go SQLite driver; CGO_ENABLED=0 compatible
)

// Store wraps a *sql.DB connection to the railblock database. It is safe for
// concurrent use: database/sql pools connections and modernc.org/sqlite
// serializes writers via its own mutex plus BEGIN IMMEDIATE in the callers.
type Store struct {
	db *sql.DB
}

// Open creates or opens the SQLite file at path, applies the schema and tunes
// pragmatic options for durability and concurrency:
//   - journal_mode=WAL: readers don't block a single writer;
//   - busy_timeout: a concurrent writer waits rather than failing fast;
//   - synchronous=NORMAL: WAL-safe without forcing an fsync per commit;
//   - foreign_keys=ON: FK constraints enforced;
//   - _txlock=immediate: every BEGIN is BEGIN IMMEDIATE, serializing writers.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_txlock=immediate",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	// SQLite effectively serializes writes; one connection is enough and avoids
	// "database is locked" from interleaved write transactions on extra conns.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// DBTX is the union of *sql.DB and *sql.Tx query/exec methods. Every store
// method that may run inside an InTx transaction takes this interface so the
// caller can pass either the pooled connection's tx (inside a tx) or the bare
// store (outside a tx). This is critical with SetMaxOpenConns(1): a method
// that reaches for s.db while a transaction holds the single connection
// deadlocks.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// querier returns the bare *sql.DB as a DBTX for read-only, outside-tx callers.
func (s *Store) querier() DBTX { return s.db }

// ExecContext / QueryContext / QueryRowContext forward to the pooled connection
// so *Store itself satisfies DBTX for outside-tx read paths. Never call these
// from inside an InTx transaction: with SetMaxOpenConns(1) the pooled
// connection is held by the tx and the call would deadlock — pass the *sql.Tx
// instead.
func (s *Store) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}
func (s *Store) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return s.db.QueryContext(ctx, query, args...)
}
func (s *Store) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return s.db.QueryRowContext(ctx, query, args...)
}

// BeginTx starts an IMMEDIATE transaction (see Open for _txlock=immediate).
// Callers must Commit or Rollback.
func (s *Store) BeginTx(ctx context.Context) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, nil)
}

// InTx runs fn inside a single IMMEDIATE transaction. On error the transaction
// is rolled back; on success it is committed. This is the write path used by
// every mutating service method so business invariants are enforced atomically.
func (s *Store) InTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
	}()
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// NextSeq atomically returns and increments the global monotonic sequence
// counter from the meta table. The increment and the row inserts that consume
// it must run in the same transaction so the stamp reflects insertion order
// even under a later rollback.
func (s *Store) NextSeq(ctx context.Context, tx *sql.Tx) (int64, error) {
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO meta(key,value) VALUES('next_seq',1)
		 ON CONFLICT(key) DO UPDATE SET value=value+1`,
	); err != nil {
		return 0, fmt.Errorf("next_seq bump: %w", err)
	}
	var seq int64
	if err := tx.QueryRowContext(ctx,
		`SELECT value FROM meta WHERE key='next_seq'`,
	).Scan(&seq); err != nil {
		return 0, fmt.Errorf("next_seq read: %w", err)
	}
	return seq - 1, nil // return the pre-increment value just consumed
}

// Ping verifies the database is reachable.
func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}
