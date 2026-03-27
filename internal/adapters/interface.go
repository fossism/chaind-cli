package adapters

import (
	"context"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
)

// Adapter is the sealed interface that every platform must implement
type Adapter interface {
	// Lifecycle
	Platform() string
	Start(ctx context.Context) error
	Disconnect() error

	// Read
	ReadHistory(roomID string, limit int, since time.Time) ([]schema.Message, error)
	Watch(ctx context.Context, roomID string) (<-chan schema.Message, error)

	// Write
	Send(roomID, text string) (schema.Message, error)
	Reply(msgID, text string) (schema.Message, error)
	React(msgID, emoji string) error

	// Moderate
	Ban(roomID, userID, reason string) error
	Mute(roomID, userID string, d time.Duration) error
	DeleteMessage(msgID string) error
}
