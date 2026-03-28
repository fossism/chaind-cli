package adapters

import (
	"context"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/oklog/ulid/v2"
)

// MockAdapter implements Adapter for unit tests
type MockAdapter struct {
	OutboundMessages []schema.Message
	watchCh          chan schema.Message
}

func NewMockAdapter() *MockAdapter {
	return &MockAdapter{
		OutboundMessages: []schema.Message{},
		watchCh:          make(chan schema.Message, 64),
	}
}

// SendToWatch pushes a message into the channel returned by Watch.
func (m *MockAdapter) SendToWatch(msg schema.Message) {
	m.watchCh <- msg
}

func (m *MockAdapter) Platform() string {
	return "mock"
}

func (m *MockAdapter) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (m *MockAdapter) Disconnect() error {
	return nil
}

func (m *MockAdapter) ReadHistory(roomID string, limit int, since time.Time) ([]schema.Message, error) {
	return []schema.Message{}, nil
}

func (m *MockAdapter) Watch(ctx context.Context, roomID string) (<-chan schema.Message, error) {
	return m.watchCh, nil
}

func (m *MockAdapter) Send(roomID, text string) (schema.Message, error) {
	msg := schema.Message{
		ID: ulid.Make().String(),
		Platform: "mock",
		PlatformID: "mock_id",
		Content: schema.Content{Text: text},
	}
	m.OutboundMessages = append(m.OutboundMessages, msg)
	return msg, nil
}

func (m *MockAdapter) Reply(msgID, text string) (schema.Message, error) {
	return m.Send("mock_room", text)
}

func (m *MockAdapter) React(msgID, emoji string) error {
	return nil
}

func (m *MockAdapter) Ban(roomID, userID, reason string) error {
	return nil
}

func (m *MockAdapter) Mute(roomID, userID string, d time.Duration) error {
	return nil
}

func (m *MockAdapter) DeleteMessage(msgID string) error {
	return nil
}
