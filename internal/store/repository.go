package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
)

// GetRecentMessages retrieves the most recent messages from the database
func (s *Store) GetRecentMessages(ctx context.Context, limit int) ([]schema.Message, error) {
	query := `
		SELECT id, platform, platform_id, room_id, author_id, text, timestamp, root_id, parent_id, read, edited, deleted
		FROM messages
		ORDER BY id DESC -- relies on ULID lexicographical sorting instead of timestamp index overhead
		LIMIT ?
	`

	type flatMsg struct {
		ID         string  `db:"id"`
		Platform   string  `db:"platform"`
		PlatformID string  `db:"platform_id"`
		RoomID     *string `db:"room_id"`
		AuthorID   *string `db:"author_id"`
		Text       *string `db:"text"`
		Timestamp  *string `db:"timestamp"`
		RootID     *string `db:"root_id"`
		ParentID   *string `db:"parent_id"`
		Read       *bool   `db:"read"`
		Edited     *bool   `db:"edited"`
		Deleted    *bool   `db:"deleted"`
	}

	var flatMsgs []flatMsg
	// Use read pool
	err := s.db.SelectContext(ctx, &flatMsgs, query, limit)
	if err != nil {
		return nil, err
	}

	var msgs []schema.Message
	for _, f := range flatMsgs {
		m := schema.Message{
			ID:         f.ID,
			Platform:   f.Platform,
			PlatformID: f.PlatformID,
			RootID:     f.RootID,
			ParentID:   f.ParentID,
		}
		if f.Read != nil {
			m.Read = *f.Read
		}
		if f.Edited != nil {
			m.Edited = *f.Edited
		}
		if f.Deleted != nil {
			m.Deleted = *f.Deleted
		}
		if f.RoomID != nil {
			m.Room.ID = *f.RoomID
		}
		if f.AuthorID != nil {
			m.Author.ID = *f.AuthorID
		}
		if f.Text != nil {
			m.Content.Text = *f.Text
			m.Content.Type = "text"
		}
		msgs = append(msgs, m)
	}

	return msgs, nil
}

// Token represents an IPC authorization capability.
type Token struct {
	Name     string `db:"name"`
	Tier     int    `db:"tier"`
	Rooms    string `db:"rooms"`
	PiiScrub string `db:"pii_scrub"`
	Expires  string `db:"expires"`
	Revoked  bool   `db:"revoked"`
}

// HashToken returns the SHA256 hex digest stored in the DB.
// Raw secrets are never persisted; only the hash is used as lookup key.
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func isHashedName(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// IsExpired reports whether the token past its Expires timestamp.
// Empty or unparsable Expires means non-expiring (legacy rows).
func (t *Token) IsExpired(now time.Time) bool {
	if t == nil || t.Expires == "" {
		return false
	}
	exp, err := time.Parse(time.RFC3339, t.Expires)
	if err != nil {
		return false
	}
	return now.After(exp)
}

// GetToken validates and retrieves an IPC capability token from the authoritative SQLite registry.
// Input is the raw bearer secret; it is hashed before lookup.
// Legacy plaintext rows are still honored as fallback for migration.
func (s *Store) GetToken(ctx context.Context, name string) (*Token, error) {
	query := `SELECT name, tier, rooms, pii_scrub, expires, revoked FROM tokens WHERE name = ?`
	var t Token
	// Use read pool
	hashed := name
	if !isHashedName(name) {
		hashed = HashToken(name)
	}
	if err := s.db.GetContext(ctx, &t, query, hashed); err == nil {
		return &t, nil
	} else if hashed != name {
		// Fallback for pre-hash rows created before hashed storage.
		if err2 := s.db.GetContext(ctx, &t, query, name); err2 == nil {
			return &t, nil
		}
	}
	// Preserve original not-found error semantics for unknown tokens.
	var t2 Token
	err := s.db.GetContext(ctx, &t2, query, hashed)
	if err != nil {
		return nil, err
	}
	return &t2, nil
}

func (s *Store) SaveToken(ctx context.Context, t Token) error {
	// Never persist raw bearer secrets: hash plaintext names on write.
	// Already-hashed (64 hex) values pass through for idempotent updates.
	if !isHashedName(t.Name) {
		t.Name = HashToken(t.Name)
	}
	query := `
		INSERT INTO tokens (name, tier, rooms, pii_scrub, expires, revoked) 
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			tier = excluded.tier,
			rooms = excluded.rooms,
			pii_scrub = excluded.pii_scrub,
			expires = excluded.expires,
			revoked = excluded.revoked
	`
	_, err := s.writeDB.ExecContext(ctx, query, t.Name, t.Tier, t.Rooms, t.PiiScrub, t.Expires, t.Revoked)
	return err
}

func (s *Store) ListTokens(ctx context.Context) ([]Token, error) {
	var tokens []Token
	err := s.db.SelectContext(ctx, &tokens, "SELECT name, tier, rooms, pii_scrub, expires, revoked FROM tokens")
	return tokens, err
}

func (s *Store) RevokeToken(ctx context.Context, name string) error {
	_, err := s.writeDB.ExecContext(ctx, "UPDATE tokens SET revoked = 1 WHERE name = ?", name)
	return err
}

func (s *Store) GetSyncState(ctx context.Context, platform, key string) (string, error) {
	query := `SELECT value FROM sync_state WHERE platform = ? AND key = ?`
	var val string
	err := s.db.GetContext(ctx, &val, query, platform, key)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (s *Store) SetSyncState(ctx context.Context, platform, key, value string) error {
	query := `
		INSERT INTO sync_state (platform, key, value) VALUES (?, ?, ?)
		ON CONFLICT(platform, key) DO UPDATE SET value = excluded.value
	`
	// Use write connection directly blockingly for sync state to ensure durability
	_, err := s.writeDB.ExecContext(ctx, query, platform, key, value)
	return err
}

func (s *Store) SaveRoom(ctx context.Context, r schema.Room) error {
	query := `
		INSERT INTO rooms (id, platform, platform_id, alias, name, type) 
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(platform, platform_id) DO UPDATE SET
			alias = excluded.alias,
			name = excluded.name
	`
	_, err := s.writeDB.ExecContext(ctx, query, r.ID, r.PlatformID /* assuming Platform is derived from ID prefix for now or pass separately */, r.PlatformID, r.Alias, r.Name, r.Type)
	return err
}

func (s *Store) SaveUser(ctx context.Context, u schema.Author) error {
	query := `
		INSERT INTO users (id, platform, platform_id, username, display_name, is_bot) 
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(platform, platform_id) DO UPDATE SET
			username = excluded.username,
			display_name = excluded.display_name
	`
	_, err := s.writeDB.ExecContext(ctx, query, u.ID, u.PlatformID, u.PlatformID, u.Username, u.DisplayName, u.IsBot)
	return err
}

// GetMessage retrieves a single message by its canonical ID
func (s *Store) GetMessage(ctx context.Context, id string) (*schema.Message, error) {
	query := `
		SELECT id, platform, platform_id, room_id, author_id, text, timestamp, root_id, parent_id, read, edited, deleted
		FROM messages
		WHERE id = ? LIMIT 1
	`

	type flatMsg struct {
		ID         string  `db:"id"`
		Platform   string  `db:"platform"`
		PlatformID string  `db:"platform_id"`
		RoomID     *string `db:"room_id"`
		AuthorID   *string `db:"author_id"`
		Text       *string `db:"text"`
		Timestamp  *string `db:"timestamp"`
		RootID     *string `db:"root_id"`
		ParentID   *string `db:"parent_id"`
		Read       *bool   `db:"read"`
		Edited     *bool   `db:"edited"`
		Deleted    *bool   `db:"deleted"`
	}

	var f flatMsg
	err := s.db.GetContext(ctx, &f, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("message not found")
		}
		return nil, err
	}

	msg := &schema.Message{
		ID:         f.ID,
		Platform:   f.Platform,
		PlatformID: f.PlatformID,
		RootID:     f.RootID,
		ParentID:   f.ParentID,
	}
	if f.Read != nil {
		msg.Read = *f.Read
	}
	if f.Edited != nil {
		msg.Edited = *f.Edited
	}
	if f.Deleted != nil {
		msg.Deleted = *f.Deleted
	}
	if f.RoomID != nil {
		msg.Room.ID = *f.RoomID
	}
	if f.AuthorID != nil {
		msg.Author.ID = *f.AuthorID
	}
	if f.Text != nil {
		msg.Content.Text = *f.Text
		msg.Content.Type = "text"
	}

	return msg, nil
}
