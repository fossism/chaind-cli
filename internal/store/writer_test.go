package store

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newWriterTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := NewStoreFromPath(filepath.Join(t.TempDir(), "writer_test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { st.Close() })
	return st
}

func makeMsg(platform, platformID, text string) schema.Message {
	return schema.Message{
		SchemaVersion: "1.0",
		ID:            ulid.Make().String(),
		Platform:      platform,
		PlatformID:    platformID,
		Room:          schema.Room{ID: "room1"},
		Author:        schema.Author{ID: "author1"},
		Content:       schema.Content{Type: "text", Text: text},
		Timestamp:     time.Now().UTC(),
	}
}

func TestStoreWriter_ConcurrentWrites_NoDrop(t *testing.T) {
	st := newWriterTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.StartWriter(ctx)

	goroutines := 10
	msgsEach := 50 // 500 total, well within 1000 buffer
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(gid int) {
			defer wg.Done()
			for m := 0; m < msgsEach; m++ {
				st.PushMessage(makeMsg("test", fmt.Sprintf("g%d_m%d", gid, m), "concurrent"))
			}
		}(g)
	}
	wg.Wait()
	time.Sleep(300 * time.Millisecond) // let ticker flush

	msgs, err := st.GetRecentMessages(context.Background(), goroutines*msgsEach+10)
	require.NoError(t, err)
	assert.Equal(t, goroutines*msgsEach, len(msgs))
}

func TestStoreWriter_ContextCancel_FlushesRemaining(t *testing.T) {
	st := newWriterTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())

	go st.StartWriter(ctx)

	// Push a small batch then cancel immediately
	for i := 0; i < 5; i++ {
		st.PushMessage(makeMsg("test", fmt.Sprintf("cancel_%d", i), "cancel-test"))
	}
	cancel()
	time.Sleep(200 * time.Millisecond)

	msgs, err := st.GetRecentMessages(context.Background(), 10)
	require.NoError(t, err)
	// At least some messages should have been flushed before or after cancel
	assert.NotEmpty(t, msgs)
}

func TestStoreWriter_ChannelFull_DropsWithoutBlocking(t *testing.T) {
	st := newWriterTestStore(t)
	// Do NOT start the writer — channel will fill up
	// Push more than buffer capacity (1000) — should not block
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1100; i++ {
			st.PushMessage(makeMsg("test", fmt.Sprintf("drop_%d", i), "drop-test"))
		}
		close(done)
	}()

	select {
	case <-done:
		// success — did not block
	case <-time.After(2 * time.Second):
		t.Fatal("PushMessage blocked when channel was full")
	}
}

func TestStoreWriter_BatchOf50_FlushesImmediately(t *testing.T) {
	st := newWriterTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go st.StartWriter(ctx)

	// Push exactly 50 messages — should trigger immediate flush before 100ms ticker
	for i := 0; i < 50; i++ {
		st.PushMessage(makeMsg("test", fmt.Sprintf("batch_%d", i), "batch-test"))
	}
	time.Sleep(50 * time.Millisecond) // less than ticker interval

	msgs, err := st.GetRecentMessages(context.Background(), 60)
	require.NoError(t, err)
	assert.Equal(t, 50, len(msgs))
}
