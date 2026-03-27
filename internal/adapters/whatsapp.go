package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/fossism/chaind-cli/internal/store"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog/log"

	_ "github.com/glebarez/go-sqlite"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type WhatsAppAdapter struct {
	client   *whatsmeow.Client
	store    *store.Store
	mu       sync.RWMutex
	watchers map[string][]chan schema.Message
}

func NewWhatsAppAdapter(st *store.Store, enabled, acceptedRisk bool) (*WhatsAppAdapter, error) {
	if !enabled || !acceptedRisk {
		return nil, fmt.Errorf("whatsapp adapter is disabled or risk not explicitly accepted")
	}

	home, _ := os.UserHomeDir()
	dbDir := filepath.Join(home, ".local", "share", "chaind")
	dbPath := filepath.Join(dbDir, "whatsapp.db")

	dbLog := waLog.Stdout("Database", "WARN", true)
	container, err := sqlstore.New(context.Background(), "sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)", dbLog)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to whatsapp store: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, err
	}

	clientLog := waLog.Stdout("Client", "WARN", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	return &WhatsAppAdapter{
		client:   client,
		store:    st,
		watchers: make(map[string][]chan schema.Message),
	}, nil
}

func (w *WhatsAppAdapter) Platform() string {
	return "whatsapp"
}

func (w *WhatsAppAdapter) Start(ctx context.Context) error {
	log.Info().Msg("WhatsApp sync loop starting...")

	w.client.AddEventHandler(func(evt interface{}) {
		w.handleEvent(evt)
	})

	if w.client.Store.ID == nil {
		// New device pairing via QR
		qrChan, _ := w.client.GetQRChannel(context.Background())
		err := w.client.Connect()
		if err != nil {
			return err
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Println("\nWhatsApp wants to connect! Please scan this QR code via Linked Devices:")
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println()
			} else {
				log.Info().Msgf("WhatsApp Auth Event: %s", evt.Event)
			}
		}
	} else {
		// Already logged in, just connect
		if err := w.client.Connect(); err != nil {
			return err
		}
	}

	<-ctx.Done()
	log.Info().Msg("WhatsApp sync loop stopping...")
	w.client.Disconnect()
	return nil
}

func (w *WhatsAppAdapter) handleEvent(rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.Message:
		text := evt.Message.GetConversation()
		if text == "" && evt.Message.GetExtendedTextMessage() != nil {
			text = evt.Message.GetExtendedTextMessage().GetText()
		}

		if text == "" {
			return
		}

		senderID := evt.Info.Sender.User
		roomID := evt.Info.Chat.User
		if evt.Info.IsGroup {
			roomID = evt.Info.Chat.User // The JID user part acts as group ID
		}

		msg := schema.Message{
			SchemaVersion: "1.0",
			ID:            ulid.Make().String(),
			Platform:      "whatsapp",
			PlatformID:    evt.Info.ID,
			Room:          schema.Room{ID: fmt.Sprintf("whatsapp:%s", roomID)},
			Author:        schema.Author{ID: senderID},
			Content:       schema.Content{Type: "text", Text: text},
			Timestamp:     evt.Info.Timestamp.UTC(),
		}

		w.store.PushMessage(msg)

		w.mu.RLock()
		defer w.mu.RUnlock()
		for _, ch := range w.watchers[roomID] {
			select {
			case ch <- msg:
			default:
			}
		}
		for _, ch := range w.watchers[""] {
			select {
			case ch <- msg:
			default:
			}
		}
	}
}

func (w *WhatsAppAdapter) Disconnect() error {
	w.client.Disconnect()
	return nil
}

func (w *WhatsAppAdapter) ReadHistory(roomID string, limit int, since time.Time) ([]schema.Message, error) {
	return nil, fmt.Errorf("not supported on whatsapp natively")
}

func (w *WhatsAppAdapter) Watch(ctx context.Context, roomID string) (<-chan schema.Message, error) {
	ch := make(chan schema.Message, 100)

	w.mu.Lock()
	w.watchers[roomID] = append(w.watchers[roomID], ch)
	w.mu.Unlock()

	go func() {
		<-ctx.Done()
		w.mu.Lock()
		defer w.mu.Unlock()

		var updated []chan schema.Message
		for _, watchCh := range w.watchers[roomID] {
			if watchCh != ch {
				updated = append(updated, watchCh)
			}
		}
		w.watchers[roomID] = updated
		close(ch)
	}()

	return ch, nil
}

func (w *WhatsAppAdapter) Send(roomID, text string) (schema.Message, error) {
	return schema.Message{}, fmt.Errorf("whatsapp send not formally exposed without full JID maps")
}

func (w *WhatsAppAdapter) Reply(msgID, text string) (schema.Message, error) {
	return schema.Message{}, fmt.Errorf("whatsapp reply not yet implemented")
}

func (w *WhatsAppAdapter) React(msgID, emoji string) error {
	return fmt.Errorf("whatsapp react not yet implemented")
}

func (w *WhatsAppAdapter) Ban(roomID, userID, reason string) error {
	return fmt.Errorf("whatsapp moderation not supported")
}

func (w *WhatsAppAdapter) Mute(roomID, userID string, d time.Duration) error {
	return fmt.Errorf("whatsapp moderation not supported")
}

func (w *WhatsAppAdapter) DeleteMessage(msgID string) error {
	return fmt.Errorf("whatsapp delete not supported")
}
