package schema

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMessageSerialization(t *testing.T) {
	html := "<b>hello</b>"
	msg := Message{
		SchemaVersion: "1.0",
		ID:            "01ABCDEF",
		Platform:      "telegram",
		PlatformID:    "12345",
		Room:          Room{ID: "telegram:67890", Name: "Test Room", Type: "group"},
		Author:        Author{ID: "user1", DisplayName: "Riya", IsBot: false},
		Content: Content{
			Type: "text",
			Text: "Hello World",
			HTML: &html,
			Attachments: []Attachment{
				{URI: "file:///tmp/photo.jpg", MimeType: "image/jpeg", Size: 1024, Filename: "photo.jpg"},
			},
		},
		Timestamp: time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC),
		Read:      true,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Platform != "telegram" {
		t.Errorf("Expected telegram, got %s", decoded.Platform)
	}

	if len(decoded.Content.Attachments) != 1 {
		t.Errorf("Expected 1 attachment, got %d", len(decoded.Content.Attachments))
	}

	if decoded.Content.Attachments[0].MimeType != "image/jpeg" {
		t.Errorf("Expected image/jpeg, got %s", decoded.Content.Attachments[0].MimeType)
	}

	if decoded.Content.Attachments[0].Size != 1024 {
		t.Errorf("Expected size 1024, got %d", decoded.Content.Attachments[0].Size)
	}
}

func TestAttachmentJSON(t *testing.T) {
	a := Attachment{
		URI:      "file:///media/voice.ogg",
		MimeType: "audio/ogg",
		Size:     2048,
		Filename: "voice.ogg",
	}

	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Failed to marshal attachment: %v", err)
	}

	var decoded Attachment
	json.Unmarshal(data, &decoded)

	if decoded.URI != a.URI {
		t.Errorf("URI mismatch: %s vs %s", decoded.URI, a.URI)
	}
	if decoded.Filename != "voice.ogg" {
		t.Errorf("Filename mismatch: %s", decoded.Filename)
	}
}

func TestMessageNilAttachments(t *testing.T) {
	msg := Message{
		ID:       "01XYZ",
		Platform: "matrix",
		Content:  Content{Type: "text", Text: "plain"},
	}

	data, _ := json.Marshal(msg)
	var decoded Message
	json.Unmarshal(data, &decoded)

	if decoded.Content.Attachments != nil {
		t.Errorf("Expected nil attachments, got %v", decoded.Content.Attachments)
	}
}
