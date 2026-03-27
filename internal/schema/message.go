package schema

import "time"

// Message represents the 1.0 canonical schema for unified messaging
type Message struct {
	SchemaVersion string    `json:"schema_version"`
	ID            string    `json:"id"`          // ULID
	Platform      string    `json:"platform"`    // matrix | telegram
	PlatformID    string    `json:"platform_id"` // Native message ID
	Room          Room      `json:"room"`
	Author        Author    `json:"author"`
	Content       Content   `json:"content"`
	RootID        *string   `json:"root_id"`   // ULID or null
	ParentID      *string   `json:"parent_id"` // ULID or null
	Edited        bool      `json:"edited"`
	Deleted       bool      `json:"deleted"`
	Timestamp     time.Time `json:"timestamp"`
	Read          bool      `json:"read"`
	Meta          any       `json:"meta"` // Platform specific extras
}

type Room struct {
	ID         string `json:"id"`
	PlatformID string `json:"platform_id"`
	Alias      string `json:"alias"`
	Name       string `json:"name"`
	Type       string `json:"type"` // group | direct | channel
}

type Author struct {
	ID          string `json:"id"`
	PlatformID  string `json:"platform_id"`
	DisplayName string `json:"display_name"`
	Username    string `json:"username"`
	IsBot       bool   `json:"is_bot"`
}

type Content struct {
	Type        string   `json:"type"` // text|image|file|audio|sticker|event
	Text        string   `json:"text"`
	HTML        *string  `json:"html"`
	Attachments []string `json:"attachments"` // URIs or IDs
}
