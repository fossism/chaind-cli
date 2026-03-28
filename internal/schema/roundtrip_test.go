package schema

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func roundTrip(t *testing.T, msg Message) Message {
	t.Helper()
	b, err := json.Marshal(msg)
	require.NoError(t, err)
	var out Message
	require.NoError(t, json.Unmarshal(b, &out))
	return out
}

func TestMessage_RoundTrip_NilPointers(t *testing.T) {
	msg := Message{
		SchemaVersion: "1.0",
		ID:            "01ARZ3NDEKTSV4RRFFQ69G5FAV",
		Platform:      "matrix",
		Timestamp:     time.Now().UTC(),
	}
	out := roundTrip(t, msg)
	assert.Nil(t, out.RootID)
	assert.Nil(t, out.ParentID)
}

func TestMessage_RoundTrip_NonNilHTML(t *testing.T) {
	html := "<b>hello</b>"
	msg := Message{
		SchemaVersion: "1.0",
		Content:       Content{Type: "text", Text: "hello", HTML: &html},
	}
	out := roundTrip(t, msg)
	require.NotNil(t, out.Content.HTML)
	assert.Equal(t, html, *out.Content.HTML)
}

func TestMessage_RoundTrip_EmptyAttachments(t *testing.T) {
	msg := Message{
		SchemaVersion: "1.0",
		Content:       Content{Type: "text", Attachments: []Attachment{}},
	}
	out := roundTrip(t, msg)
	// empty slice may unmarshal as nil — both are acceptable empty states
	assert.Empty(t, out.Content.Attachments)
}

func TestMessage_RoundTrip_SchemaVersionPreserved(t *testing.T) {
	msg := Message{SchemaVersion: "1.0"}
	out := roundTrip(t, msg)
	assert.Equal(t, "1.0", out.SchemaVersion)
}

func TestMessage_RoundTrip_FullMessage(t *testing.T) {
	rootID := "ROOT01"
	parentID := "PARENT01"
	html := "<i>world</i>"
	msg := Message{
		SchemaVersion: "1.0",
		ID:            "MSG01",
		Platform:      "telegram",
		PlatformID:    "tg_123",
		Room:          Room{ID: "r1", Name: "General", Type: "group"},
		Author:        Author{ID: "u1", DisplayName: "Riya", IsBot: false},
		Content:       Content{Type: "text", Text: "hello world", HTML: &html},
		RootID:        &rootID,
		ParentID:      &parentID,
		Edited:        true,
		Timestamp:     time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	out := roundTrip(t, msg)
	assert.Equal(t, msg.ID, out.ID)
	assert.Equal(t, msg.Platform, out.Platform)
	assert.Equal(t, msg.Room.Name, out.Room.Name)
	assert.Equal(t, msg.Author.DisplayName, out.Author.DisplayName)
	assert.Equal(t, msg.Content.Text, out.Content.Text)
	require.NotNil(t, out.RootID)
	assert.Equal(t, rootID, *out.RootID)
	require.NotNil(t, out.ParentID)
	assert.Equal(t, parentID, *out.ParentID)
	assert.True(t, out.Edited)
}
