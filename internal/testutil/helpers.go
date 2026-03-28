package testutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/fossism/chaind-cli/internal/daemon"
	"github.com/fossism/chaind-cli/internal/ipc"
	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/fossism/chaind-cli/internal/store"
	"github.com/oklog/ulid/v2"
)

// NewTestStore opens a pure-Go SQLite DB at t.TempDir()/test.db, runs all migrations,
// and registers t.Cleanup to close it.
func NewTestStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	st, err := store.NewStoreFromPath(dbPath)
	if err != nil {
		t.Fatalf("NewTestStore: failed to open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// NewTestIPCServer starts an IPCServer on a temp Unix socket, registers t.Cleanup to shut it
// down, and returns the socket path and an HTTP client pre-wired to dial it.
func NewTestIPCServer(t *testing.T, st *store.Store, router *daemon.AdapterRouter) (string, *http.Client) {
	t.Helper()
	sockPath := filepath.Join(t.TempDir(), "test.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("NewTestIPCServer: failed to listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	srv := ipc.NewIPCServer(st, router)

	go func() {
		if err := srv.StartOnListener(ctx, ln); err != nil {
			// http.ErrServerClosed is expected on shutdown
		}
	}()

	t.Cleanup(func() {
		cancel()
		ln.Close()
	})

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sockPath)
			},
		},
	}
	return sockPath, client
}

// NewTestToken inserts a token row into st and returns the token name string.
func NewTestToken(t *testing.T, st *store.Store, tier int, rooms, piiScrub string) string {
	t.Helper()
	name := fmt.Sprintf("test-token-%s", ulid.Make().String())
	_, err := st.DB().Exec(
		`INSERT INTO tokens (name, tier, rooms, pii_scrub, expires, revoked) VALUES (?, ?, ?, ?, ?, ?)`,
		name, tier, rooms, piiScrub, "2099-01-01T00:00:00Z", false,
	)
	if err != nil {
		t.Fatalf("NewTestToken: failed to insert token: %v", err)
	}
	return name
}

// MakeMessage returns a schema.Message with a fresh ULID, SchemaVersion "1.0", and current timestamp.
func MakeMessage(platform, roomID, text string) schema.Message {
	return schema.Message{
		SchemaVersion: "1.0",
		ID:            ulid.Make().String(),
		Platform:      platform,
		PlatformID:    ulid.Make().String(),
		Room:          schema.Room{ID: roomID, PlatformID: roomID},
		Author:        schema.Author{ID: ulid.Make().String()},
		Content:       schema.Content{Type: "text", Text: text},
		Timestamp:     time.Now().UTC(),
	}
}
