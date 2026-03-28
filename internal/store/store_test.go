package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := NewStoreFromPath(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	return st
}

func TestStore_Migration_AllTablesExist(t *testing.T) {
	st := newTestStore(t)
	tables := []string{
		"messages", "rooms", "users", "tokens",
		"approval_queue", "outbox", "sync_state",
		"sync_cursors", "modlog", "access_log", "optout",
	}
	for _, tbl := range tables {
		var name string
		err := st.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl,
		).Scan(&name)
		require.NoError(t, err, "table %q should exist", tbl)
		assert.Equal(t, tbl, name)
	}
}

func TestStore_Migration_FTS5AndTriggersExist(t *testing.T) {
	st := newTestStore(t)

	// FTS5 virtual table
	var name string
	err := st.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='messages_fts'",
	).Scan(&name)
	require.NoError(t, err, "messages_fts virtual table should exist")

	// Triggers
	for _, trig := range []string{"messages_ai", "messages_ad", "messages_au"} {
		err := st.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='trigger' AND name=?", trig,
		).Scan(&name)
		require.NoError(t, err, "trigger %q should exist", trig)
	}
}

func TestStore_Migration_Idempotent(t *testing.T) {
	st := newTestStore(t)
	// Running the schema SQL a second time must not error
	_, err := st.writeDB.Exec(SchemaSQL)
	assert.NoError(t, err)
}

func TestStore_WALModeActive(t *testing.T) {
	st := newTestStore(t)
	var mode string
	err := st.db.QueryRow("PRAGMA journal_mode").Scan(&mode)
	require.NoError(t, err)
	assert.Equal(t, "wal", mode)
}
