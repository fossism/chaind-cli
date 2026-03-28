package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fossism/chaind-cli/internal/schema"
	_ "github.com/glebarez/go-sqlite"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

type Store struct {
	db      *sqlx.DB // Read pool
	writeDB *sqlx.DB // Write connection
	writer  *StoreWriter
}

// NewStoreFromPath opens (or creates) a Store at the given path.
// Used by tests to open a t.TempDir()-isolated database.
func NewStoreFromPath(dbPath string) (*Store, error) {
	// Read connection pool
	db, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite read db: %w", err)
	}
	db.Exec("PRAGMA journal_mode=WAL;")
	db.Exec("PRAGMA foreign_keys=ON;")
	db.SetMaxOpenConns(10)

	// Write connection
	writeDB, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite write db: %w", err)
	}
	writeDB.Exec("PRAGMA journal_mode=WAL;")
	writeDB.Exec("PRAGMA foreign_keys=ON;")
	writeDB.SetMaxOpenConns(1)

	if err := writeDB.PingContext(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping write db: %w", err)
	}

	log.Info().Str("path", dbPath).Msg("Connected to chaind local store")

	s := &Store{db: db, writeDB: writeDB}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate db: %w", err)
	}

	s.writer = NewStoreWriter(writeDB)
	return s, nil
}

func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home dir: %w", err)
	}

	dbDir := filepath.Join(home, ".local", "share", "chaind")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	return NewStoreFromPath(filepath.Join(dbDir, "messages.db"))
}

func (s *Store) migrate() error {
	_, err := s.writeDB.Exec(SchemaSQL)
	if err != nil {
		log.Error().Err(err).Msg("Database migration failed")
		return err
	}
	log.Debug().Msg("Database schema initialized")
	return nil
}

// StartWriter launches the dedicated thread for SQLite inserts.
func (s *Store) StartWriter(ctx context.Context) {
	s.writer.Run(ctx)
}

// PushMessage delegates a write securely without blocking the adapter.
func (s *Store) PushMessage(msg schema.Message) {
	s.writer.Push(msg)
}

// DB returns the read connection pool for direct queries (e.g., FTS5 search).
func (s *Store) DB() *sqlx.DB {
	return s.db
}

func (s *Store) Close() error {
	s.writeDB.Close()
	return s.db.Close()
}

// GetCursor retrieves the last sync timestamp for a specific platform and room to allow offline backfill.
func (s *Store) GetCursor(ctx context.Context, platform, roomID string) (int64, error) {
	var ts int64
	err := s.db.QueryRowContext(ctx, "SELECT cursor_timestamp FROM sync_cursors WHERE platform = ? AND room_id = ?", platform, roomID).Scan(&ts)
	if err != nil {
		return 0, err
	}
	return ts, nil
}

// SaveCursor persists the latest cursor immediately to the write pool.
func (s *Store) SaveCursor(ctx context.Context, platform, roomID string, ts int64) error {
	_, err := s.writeDB.ExecContext(ctx, `
		INSERT INTO sync_cursors (platform, room_id, cursor_timestamp) 
		VALUES (?, ?, ?)
		ON CONFLICT(platform, room_id) 
		DO UPDATE SET cursor_timestamp=excluded.cursor_timestamp`,
		platform, roomID, ts)
	return err
}
