package daemon

import (
	"context"
	"time"

	"github.com/fossism/chaind-cli/internal/store"
	"github.com/rs/zerolog/log"
)

func StartScheduler(ctx context.Context, db *store.Store, router *AdapterRouter) {
	log.Info().Msg("Starting outbox scheduler goroutine (30s intervals)")
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				processOutbox(db, router)
			}
		}
	}()
}

func processOutbox(db *store.Store, router *AdapterRouter) {
	type outboxItem struct {
		ID          string `db:"id"`
		Platform    string `db:"platform"`
		RoomID      string `db:"room_id"`
		Content     string `db:"content"`
		ScheduledAt string `db:"scheduled_at"`
	}

	var items []outboxItem
	// Fetch items where scheduled_at <= now
	err := db.DB().Select(&items, "SELECT id, platform, room_id, content, scheduled_at FROM outbox WHERE scheduled_at <= datetime('now')")
	if err != nil {
		log.Error().Err(err).Msg("Failed to poll outbox")
		return
	}

	for _, item := range items {
		log.Info().Str("id", item.ID).Msg("Executing scheduled message")
		_, err := router.Send(item.Platform, item.RoomID, item.Content)
		if err != nil {
			log.Error().Err(err).Str("id", item.ID).Msg("Failed outbox delivery")
			// Keep it in outbox or move to failed? Let's just delete anyway so it doesn't loop infinitely, or maybe we leave it. Let's delete.
		}
		db.DB().Exec("DELETE FROM outbox WHERE id = ?", item.ID)
	}
}
