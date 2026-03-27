package store

import (
	"context"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog/log"
)

type StoreWriter struct {
	ch chan schema.Message
	db *sqlx.DB
}

func NewStoreWriter(db *sqlx.DB) *StoreWriter {
	return &StoreWriter{
		ch: make(chan schema.Message, 1000), // Buffered channel per spec
		db: db,
	}
}

func (sw *StoreWriter) Run(ctx context.Context) {
	batch := make([]schema.Message, 0, 50)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Flush remaining before exit
			if len(batch) > 0 {
				sw.flush(batch)
			}
			return
		case msg := <-sw.ch:
			batch = append(batch, msg)
			if len(batch) >= 50 {
				sw.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				sw.flush(batch)
				batch = batch[:0]
			}
		}
	}
}

func (sw *StoreWriter) Push(msg schema.Message) {
	select {
	case sw.ch <- msg:
	default:
		log.Warn().Str("msg_id", msg.ID).Msg("store writer buffer full - message dropped")
	}
}

func (sw *StoreWriter) flush(batch []schema.Message) {
	if len(batch) == 0 {
		return
	}

	tx, err := sw.db.Beginx()
	if err != nil {
		log.Error().Err(err).Msg("failed to begin store flush transaction")
		return
	}

	query := `
		INSERT OR IGNORE INTO messages 
		(id, platform, platform_id, room_id, author_id, text, timestamp, root_id, parent_id, read, edited, deleted)
		VALUES (:id, :platform, :platform_id, :room_id, :author_id, :text, :timestamp, :root_id, :parent_id, :read, :edited, :deleted)
	`
	for _, m := range batch {
		// Prepare flat struct for NamedExec
		flat := map[string]interface{}{
			"id":          m.ID,
			"platform":    m.Platform,
			"platform_id": m.PlatformID,
			"room_id":     m.Room.ID,
			"author_id":   m.Author.ID,
			"text":        m.Content.Text,
			"timestamp":   m.Timestamp,
			"root_id":     m.RootID,
			"parent_id":   m.ParentID,
			"read":        m.Read,
			"edited":      m.Edited,
			"deleted":     m.Deleted,
		}
		_, err := tx.NamedExec(query, flat)
		if err != nil {
			log.Error().Err(err).Str("id", m.ID).Msg("failed to insert message during flush")
		}
	}

	if err := tx.Commit(); err != nil {
		log.Error().Err(err).Msg("failed to commit batch flush")
	} else {
		log.Debug().Int("count", len(batch)).Msg("Flushed message batch to disk")
	}
}
