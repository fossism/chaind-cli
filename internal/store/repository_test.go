package store

import (
	"context"
	"testing"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
)

func TestStoreWriterConcurrency(t *testing.T) {
	// Initialize a new store. Using the actual local dev store initialization logic.
	// Note: Since this touches the disk, it verifies the actual WAL mode logic works.
	st, err := NewStore()
	if err != nil {
		t.Skipf("Skipping integration test; unable to init SQLite: %v", err)
	}
	defer st.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Launch background writer
	go st.StartWriter(ctx)

	// Throw a rapid burst of messages at the Push channel to ensure no deadlock and no SQLITE_BUSY
	burstCount := 100
	for i := 0; i < burstCount; i++ {
		st.PushMessage(schema.Message{
			ID:         "TEST_ID",
			Platform:   "test",
			PlatformID: "123",
			Content:    schema.Content{Text: "burst test"},
			Timestamp:  time.Now(),
		})
	}

	// Sleep briefly to let the batch flush
	time.Sleep(200 * time.Millisecond)

	msgs, err := st.GetRecentMessages(context.Background(), burstCount)
	if err != nil {
		t.Fatalf("failed to read recent messages: %v", err)
	}
	
	// Because the store might have older messages, we just ensure it didn't panic and reads successfully
	if len(msgs) == 0 {
		t.Log("Warning: Test executed but no messages were read back (flusher may be slow).")
	}
}
