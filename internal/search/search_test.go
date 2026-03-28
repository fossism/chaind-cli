package search

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/fossism/chaind-cli/internal/store"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSearchTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.NewStoreFromPath(filepath.Join(t.TempDir(), "search_test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	return st
}

// insertSearchMsg inserts a message directly for search tests.
func insertSearchMsg(t *testing.T, st *store.Store, id, text string, ts time.Time) {
	t.Helper()
	_, err := st.DB().Exec(
		`INSERT INTO messages (id, platform, platform_id, room_id, author_id, text, timestamp, read, edited, deleted)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 0, 0, 0)`,
		id, "test", fmt.Sprintf("pid_%s", id), "room1", "author1", text, ts.Format(time.RFC3339),
	)
	require.NoError(t, err)
}

func TestSearch_ReturnsMatchingMessages(t *testing.T) {
	st := newSearchTestStore(t)
	se := NewSearchEngine(st)

	insertSearchMsg(t, st, ulid.Make().String(), "hello world", time.Now())
	insertSearchMsg(t, st, ulid.Make().String(), "goodbye world", time.Now())
	insertSearchMsg(t, st, ulid.Make().String(), "unrelated content", time.Now())

	msgs, err := se.Search(context.Background(), "hello", 10, "")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content.Text, "hello")
}

func TestSearch_NoMatch_ReturnsEmptySlice(t *testing.T) {
	st := newSearchTestStore(t)
	se := NewSearchEngine(st)

	insertSearchMsg(t, st, ulid.Make().String(), "some message", time.Now())

	msgs, err := se.Search(context.Background(), "zzznomatch", 10, "")
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestSearch_SinceFilter_ExcludesOldMessages(t *testing.T) {
	st := newSearchTestStore(t)
	se := NewSearchEngine(st)

	old := time.Now().Add(-48 * time.Hour)
	recent := time.Now()

	insertSearchMsg(t, st, ulid.Make().String(), "ancient message", old)
	insertSearchMsg(t, st, ulid.Make().String(), "ancient message", recent)

	msgs, err := se.Search(context.Background(), "ancient", 10, "24h")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	// The recent one should be returned
	assert.WithinDuration(t, recent, msgs[0].Timestamp, 5*time.Second)
}

func TestSearch_DeleteTrigger_RemovedMessageNotReturned(t *testing.T) {
	st := newSearchTestStore(t)
	se := NewSearchEngine(st)

	id := ulid.Make().String()
	insertSearchMsg(t, st, id, "deleteme content", time.Now())

	// Verify it's found first
	msgs, err := se.Search(context.Background(), "deleteme", 10, "")
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	// Delete it
	_, err = st.DB().Exec("DELETE FROM messages WHERE id = ?", id)
	require.NoError(t, err)

	// Should no longer appear
	msgs, err = se.Search(context.Background(), "deleteme", 10, "")
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestSearch_UpdateTrigger_UpdatedTextReturned(t *testing.T) {
	st := newSearchTestStore(t)
	se := NewSearchEngine(st)

	id := ulid.Make().String()
	insertSearchMsg(t, st, id, "original text", time.Now())

	// Update the text
	_, err := st.DB().Exec("UPDATE messages SET text = ? WHERE id = ?", "updated text", id)
	require.NoError(t, err)

	msgs, err := se.Search(context.Background(), "updated", 10, "")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Contains(t, msgs[0].Content.Text, "updated")
}
