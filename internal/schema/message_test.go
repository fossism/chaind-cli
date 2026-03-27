package schema

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMessageJSON(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond) // JSON drops nanoseconds usually

	msg := Message{
		SchemaVersion: "1.0",
		ID:            "01HGG9Z1N2A3B4C5D6E7F8G9H0",
		Platform:      "matrix",
		PlatformID:    "$event-id",
		Room: Room{
			ID: "matrix:!foo:example.com",
		},
		Author: Author{
			ID: "@alice:example.com",
		},
		Content: Content{
			Type: "text",
			Text: "Hello world!",
		},
		Timestamp: now,
	}

	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal msg: %v", err)
	}

	var parsed Message
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("failed to unmarshal msg: %v", err)
	}

	if parsed.ID != msg.ID {
		t.Errorf("expected ID %s, got %s", msg.ID, parsed.ID)
	}
	if parsed.Platform != msg.Platform {
		t.Errorf("expected Platform %s, got %s", msg.Platform, parsed.Platform)
	}
	if parsed.Content.Text != msg.Content.Text {
		t.Errorf("expected text %s, got %s", msg.Content.Text, parsed.Content.Text)
	}
	if !parsed.Timestamp.Equal(now) {
		t.Errorf("expected timestamp %v, got %v", now, parsed.Timestamp)
	}
}
