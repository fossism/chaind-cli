package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/fossism/chaind-cli/internal/store"
	"github.com/rs/zerolog/log"
)

type WhatsAppAdapter struct {
	store *store.Store
}

func NewWhatsAppAdapter(st *store.Store, enabled, acceptedRisk bool) (*WhatsAppAdapter, error) {
	if !enabled || !acceptedRisk {
		return nil, fmt.Errorf("whatsapp adapter is disabled or risk not explicitly accepted")
	}
	return &WhatsAppAdapter{store: st}, nil
}

func (w *WhatsAppAdapter) Platform() string {
	return "whatsapp"
}

func (w *WhatsAppAdapter) Start(ctx context.Context) error {
	log.Info().Msg("WhatsApp adapter placeholder started...")
	<-ctx.Done()
	return nil
}

func (w *WhatsAppAdapter) Disconnect() error {
	return nil
}

func (w *WhatsAppAdapter) ReadHistory(roomID string, limit int, since time.Time) ([]schema.Message, error) {
	return nil, fmt.Errorf("not supported on whatsapp yet")
}

func (w *WhatsAppAdapter) Watch(ctx context.Context, roomID string) (<-chan schema.Message, error) {
	return nil, fmt.Errorf("not supported on whatsapp yet")
}

func (w *WhatsAppAdapter) Send(roomID, text string) (schema.Message, error) {
	return schema.Message{}, fmt.Errorf("whatsapp send not yet implemented")
}

func (w *WhatsAppAdapter) Reply(msgID, text string) (schema.Message, error) {
	return schema.Message{}, fmt.Errorf("whatsapp reply not yet implemented")
}

func (w *WhatsAppAdapter) React(msgID, emoji string) error {
	return fmt.Errorf("whatsapp react not yet implemented")
}

func (w *WhatsAppAdapter) Ban(roomID, userID, reason string) error {
	return fmt.Errorf(" whatsapp moderation not supported")
}

func (w *WhatsAppAdapter) Mute(roomID, userID string, d time.Duration) error {
	return fmt.Errorf("whatsapp moderation not supported")
}

func (w *WhatsAppAdapter) DeleteMessage(msgID string) error {
	return fmt.Errorf("whatsapp delete not supported")
}
