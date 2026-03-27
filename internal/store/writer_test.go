package store_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/fossism/chaind-cli/internal/store"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreWriter_ConcurrentStress(t *testing.T) {
	// 200% Quality Architecture Validation 
	// The Grandest Vision claims the StoreWriter prevents SQLite locking under heavy concurrent ingestion.
	
	// Fast memory SQLite for isolation during testing
	st, err := store.NewStore() 
	require.NoError(t, err)
	defer st.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Spin up the StoreWriter queue manager
	go st.StartWriter(ctx)

	// Simulate 10 isolated AI Agents or Adapters bursting 1,000 messages each concurrently.
	agents := 10
	messagesPerAgent := 1000
	totalExpected := agents * messagesPerAgent

	var wg sync.WaitGroup
	wg.Add(agents)

	startTime := time.Now()

	for a := 0; a < agents; a++ {
		go func(agentID int) {
			defer wg.Done()
			for m := 0; m < messagesPerAgent; m++ {
				msg := schema.Message{
					SchemaVersion: "1.0",
					ID:            ulid.Make().String(),
					Platform:      "test",
					PlatformID:    fmt.Sprintf("agent_%d_msg_%d", agentID, m),
					Room:          schema.Room{ID: "test_room"},
					Author:        schema.Author{ID: fmt.Sprintf("agent_%d", agentID)},
					Content:       schema.Content{Type: "text", Text: "stress test data"},
					Timestamp:     time.Now().UTC(),
				}
				// Safe multiplexed push
				st.PushMessage(msg)
			}
		}(a)
	}

	// Wait for all producers
	wg.Wait()
	
	// Allow 500ms max for the 100ms ticker batch-flushes to clear the pipeline
	time.Sleep(500 * time.Millisecond)

	// Assertions
	duration := time.Since(startTime)
	
	msgs, err := st.GetRecentMessages(context.Background(), totalExpected + 10)
	require.NoError(t, err)

	assert.Equal(t, totalExpected, len(msgs), "StoreWriter should not drop any messages under heavy concurrency")
	t.Logf("Successfully ingested %d messages from %d concurrent writers in %s", len(msgs), agents, duration)
}
