package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRepoTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := NewStoreFromPath(filepath.Join(t.TempDir(), "repo_test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	return st
}

// insertMsg directly inserts a message into the DB for query tests.
func insertMsg(t *testing.T, st *Store, msg schema.Message) {
	t.Helper()
	_, err := st.writeDB.Exec(
		`INSERT INTO messages (id, platform, platform_id, room_id, author_id, text, timestamp, read, edited, deleted)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.Platform, msg.PlatformID, msg.Room.ID, msg.Author.ID,
		msg.Content.Text, msg.Timestamp.Format(time.RFC3339),
		msg.Read, msg.Edited, msg.Deleted,
	)
	require.NoError(t, err)
}

func TestGetRecentMessages_ReturnsAtMostLimit(t *testing.T) {
	st := newRepoTestStore(t)
	for i := 0; i < 10; i++ {
		insertMsg(t, st, schema.Message{
			ID: ulid.Make().String(), Platform: "test",
			PlatformID: fmt.Sprintf("p%d", i), Room: schema.Room{ID: "r1"},
			Author: schema.Author{ID: "a1"}, Content: schema.Content{Text: "msg"},
			Timestamp: time.Now().UTC(),
		})
	}
	msgs, err := st.GetRecentMessages(context.Background(), 5)
	require.NoError(t, err)
	assert.Len(t, msgs, 5)
}

func TestGetRecentMessages_DescendingOrder(t *testing.T) {
	st := newRepoTestStore(t)
	var ids []string
	for i := 0; i < 3; i++ {
		id := ulid.Make().String()
		ids = append(ids, id)
		time.Sleep(time.Millisecond) // ensure distinct ULIDs
		insertMsg(t, st, schema.Message{
			ID: id, Platform: "test", PlatformID: fmt.Sprintf("p%d", i),
			Room: schema.Room{ID: "r1"}, Author: schema.Author{ID: "a1"},
			Content: schema.Content{Text: "msg"}, Timestamp: time.Now().UTC(),
		})
	}
	msgs, err := st.GetRecentMessages(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, msgs, 3)
	// ULID order descending means last inserted first
	assert.Equal(t, ids[2], msgs[0].ID)
}

func TestGetMessage_KnownID(t *testing.T) {
	st := newRepoTestStore(t)
	msg := schema.Message{
		ID: ulid.Make().String(), Platform: "test", PlatformID: "p1",
		Room: schema.Room{ID: "r1"}, Author: schema.Author{ID: "a1"},
		Content: schema.Content{Text: "hello"}, Timestamp: time.Now().UTC(),
	}
	insertMsg(t, st, msg)

	got, err := st.GetMessage(context.Background(), msg.ID)
	require.NoError(t, err)
	assert.Equal(t, msg.ID, got.ID)
	assert.Equal(t, "hello", got.Content.Text)
}

func TestGetMessage_UnknownID_ReturnsError(t *testing.T) {
	st := newRepoTestStore(t)
	_, err := st.GetMessage(context.Background(), "nonexistent")
	assert.Error(t, err)
}

func TestGetToken_KnownName(t *testing.T) {
	st := newRepoTestStore(t)
	_, err := st.writeDB.Exec(
		`INSERT INTO tokens (name, tier, rooms, pii_scrub, expires, revoked) VALUES (?, ?, ?, ?, ?, ?)`,
		"mytoken", 0, "*", "email", "2099-01-01T00:00:00Z", false,
	)
	require.NoError(t, err)

	tok, err := st.GetToken(context.Background(), "mytoken")
	require.NoError(t, err)
	assert.Equal(t, 0, tok.Tier)
	assert.Equal(t, "*", tok.Rooms)
	assert.Equal(t, "email", tok.PiiScrub)
	assert.False(t, tok.Revoked)
}

func TestGetToken_UnknownName_ReturnsError(t *testing.T) {
	st := newRepoTestStore(t)
	_, err := st.GetToken(context.Background(), "ghost")
	assert.Error(t, err)
}

func TestSyncState_RoundTrip(t *testing.T) {
	st := newRepoTestStore(t)
	err := st.SetSyncState(context.Background(), "telegram", "last_update_id", "12345")
	require.NoError(t, err)

	val, err := st.GetSyncState(context.Background(), "telegram", "last_update_id")
	require.NoError(t, err)
	assert.Equal(t, "12345", val)
}

func TestSyncState_MissingKey_ReturnsEmpty(t *testing.T) {
	st := newRepoTestStore(t)
	val, err := st.GetSyncState(context.Background(), "telegram", "missing_key")
	require.NoError(t, err)
	assert.Equal(t, "", val)
}

func TestCursor_RoundTrip(t *testing.T) {
	st := newRepoTestStore(t)
	ts := int64(1700000000)
	err := st.SaveCursor(context.Background(), "matrix", "!room:example.com", ts)
	require.NoError(t, err)

	got, err := st.GetCursor(context.Background(), "matrix", "!room:example.com")
	require.NoError(t, err)
	assert.Equal(t, ts, got)
}
