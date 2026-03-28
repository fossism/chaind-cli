package search

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/fossism/chaind-cli/internal/store"
)

type SearchEngine struct {
	store *store.Store
}

func NewSearchEngine(s *store.Store) *SearchEngine {
	return &SearchEngine{store: s}
}

type flatMsg struct {
	ID         string  `db:"id"`
	Platform   string  `db:"platform"`
	PlatformID string  `db:"platform_id"`
	RoomID     string  `db:"room_id"`
	AuthorID   string  `db:"author_id"`
	Text       string  `db:"text"`
	Timestamp  string  `db:"timestamp"`
	RootID     *string `db:"root_id"`
	ParentID   *string `db:"parent_id"`
	Read       bool    `db:"read"`
	Edited     bool    `db:"edited"`
	Deleted    bool    `db:"deleted"`
}

func parseDuration(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// Search executes a full-text search against the SQLite FTS5 messages_fts virtual table.
func (se *SearchEngine) Search(ctx context.Context, query string, limit int, since string) ([]schema.Message, error) {
	sinceFilter := ""
	var args []interface{}
	args = append(args, query)

	if since != "" {
		dur, err := parseDuration(since)
		if err == nil {
			cutoff := time.Now().Add(-dur).Format(time.RFC3339)
			sinceFilter = " AND m.timestamp >= ? "
			args = append(args, cutoff)
		}
	}
	args = append(args, limit)

	sqlQuery := fmt.Sprintf(`
		SELECT m.id, m.platform, m.platform_id, m.room_id, m.author_id, m.text, m.timestamp,
		       m.root_id, m.parent_id, m.read, m.edited, m.deleted
		FROM messages m
		JOIN messages_fts fts ON m.rowid = fts.rowid
		WHERE messages_fts MATCH ? %s
		ORDER BY bm25(messages_fts)
		LIMIT ?
	`, sinceFilter)

	var flat []flatMsg
	err := se.store.DB().SelectContext(ctx, &flat, sqlQuery, args...)
	if err != nil {
		// Fallback to LIKE search if FTS5 is not available
		return se.fallbackSearch(ctx, query, limit, sinceFilter, args)
	}

	return se.mapResults(flat), nil
}

// fallbackSearch uses a simple LIKE query when FTS5 is not configured.
func (se *SearchEngine) fallbackSearch(ctx context.Context, query string, limit int, sinceFilter string, ftsArgs []interface{}) ([]schema.Message, error) {
	sqlQuery := fmt.Sprintf(`
		SELECT id, platform, platform_id, room_id, author_id, text, timestamp,
		       root_id, parent_id, read, edited, deleted
		FROM messages m
		WHERE m.text LIKE '%%' || ? || '%%' %s
		ORDER BY id DESC
		LIMIT ?
	`, sinceFilter)

	var flat []flatMsg
	err := se.store.DB().SelectContext(ctx, &flat, sqlQuery, ftsArgs...)
	if err != nil {
		return nil, err
	}

	return se.mapResults(flat), nil
}

func (se *SearchEngine) mapResults(flat []flatMsg) []schema.Message {
	var msgs []schema.Message
	for _, f := range flat {
		ts, _ := time.Parse(time.RFC3339, f.Timestamp)
		msgs = append(msgs, schema.Message{
			ID:         f.ID,
			Platform:   f.Platform,
			PlatformID: f.PlatformID,
			Room:       schema.Room{ID: f.RoomID},
			Author:     schema.Author{ID: f.AuthorID},
			Content:    schema.Content{Type: "text", Text: f.Text},
			RootID:     f.RootID,
			ParentID:   f.ParentID,
			Timestamp:  ts,
			Read:       f.Read,
			Edited:     f.Edited,
			Deleted:    f.Deleted,
		})
	}
	return msgs
}
