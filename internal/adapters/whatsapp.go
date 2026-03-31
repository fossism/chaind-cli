package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/fossism/chaind-cli/internal/store"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog/log"
	
	"google.golang.org/protobuf/proto"

	_ "github.com/glebarez/go-sqlite"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"go.mau.fi/whatsmeow/proto/waE2E"
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

		var attachments []schema.Attachment
		if img := evt.Message.GetImageMessage(); img != nil {
			text = img.GetCaption()
			attachments = append(attachments, schema.Attachment{
				URI:      "whatsapp-image", // pending active download layer
				MimeType: img.GetMimetype(),
				Size:     int64(img.GetFileLength()),
			})
		}
		if doc := evt.Message.GetDocumentMessage(); doc != nil {
			text = doc.GetCaption()
			attachments = append(attachments, schema.Attachment{
				URI:      "whatsapp-document", // pending active download layer
				MimeType: doc.GetMimetype(),
				Size:     int64(doc.GetFileLength()),
				Filename: doc.GetTitle(),
			})
		}

		if text == "" && len(attachments) == 0 {
			return
		}

		if evt.Info.Sender.IsEmpty() || evt.Info.Chat.IsEmpty() {
			log.Warn().Msg("Received WhatsApp message event with empty sender or chat info, skipping")
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
			Content:       schema.Content{Type: "text", Text: text, Attachments: attachments},
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
	if !w.client.IsConnected() {
		return schema.Message{}, fmt.Errorf("whatsapp client is not connected")
	}

	roomStr := strings.TrimPrefix(roomID, "whatsapp:")
	if !strings.Contains(roomStr, "@") {
		roomStr = roomStr + "@" + types.DefaultUserServer
	}
	jid, err := types.ParseJID(roomStr)
	if err != nil {
		return schema.Message{}, fmt.Errorf("invalid whatsapp jid format: %w", err)
	}

	log.Info().Str("room", roomID).Str("target_jid", jid.String()).Msg("WhatsApp dispatching message")

	waMsg := &waE2E.Message{
		Conversation: proto.String(text),
	}

	// Use a timeout for sending to avoid hanging
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	resp, err := w.client.SendMessage(ctx, jid, waMsg)
	if err != nil {
		return schema.Message{}, fmt.Errorf("whatsapp send failed: %w", err)
	}

	return schema.Message{
		ID:         ulid.Make().String(),
		Platform:   "whatsapp",
		PlatformID: resp.ID,
		Room:       schema.Room{ID: "whatsapp:" + jid.String()},
		Content:    schema.Content{Type: "text", Text: text},
		Timestamp:  resp.Timestamp,
	}, nil
}

func (w *WhatsAppAdapter) Reply(msgID, text string) (schema.Message, error) {
	ctx := context.Background()
	msg, err := w.store.GetMessage(ctx, msgID)
	if err != nil {
		return schema.Message{}, err
	}

	roomStr := strings.TrimPrefix(msg.Room.ID, "whatsapp:")
	chatJID, err := types.ParseJID(roomStr)
	if err != nil {
		return schema.Message{}, err
	}

	senderJID, err := types.ParseJID(msg.Author.ID)
	if err != nil {
		senderJID = chatJID
	}

	waMsg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(text),
			ContextInfo: &waE2E.ContextInfo{
				StanzaID:      proto.String(msg.PlatformID),
				Participant:   proto.String(senderJID.ToNonAD().String()),
				QuotedMessage: &waE2E.Message{Conversation: proto.String(msg.Content.Text)},
			},
		},
	}

	resp, err := w.client.SendMessage(ctx, chatJID, waMsg)
	if err != nil {
		return schema.Message{}, err
	}

	return schema.Message{
		ID:         ulid.Make().String(),
		Platform:   "whatsapp",
		PlatformID: resp.ID,
		Room:       msg.Room,
		Content:    schema.Content{Type: "text", Text: text},
		Timestamp:  resp.Timestamp,
		ParentID:   &msgID,
	}, nil
}

func (w *WhatsAppAdapter) React(msgID, emoji string) error {
	ctx := context.Background()
	msg, err := w.store.GetMessage(ctx, msgID)
	if err != nil {
		return err
	}

	roomStr := strings.TrimPrefix(msg.Room.ID, "whatsapp:")
	chatJID, err := types.ParseJID(roomStr)
	if err != nil {
		return err
	}

	senderJID, err := types.ParseJID(msg.Author.ID)
	if err != nil {
		senderJID = chatJID
	}

	reactionMsg := w.client.BuildReaction(chatJID, senderJID, msg.PlatformID, emoji)
	_, err = w.client.SendMessage(ctx, chatJID, reactionMsg)
	return err
}

func (w *WhatsAppAdapter) Ban(roomID, userID, reason string) error {
	chatJID, err := types.ParseJID(strings.TrimPrefix(roomID, "whatsapp:"))
	if err != nil {
		return err
	}
	userJID, err := types.ParseJID(userID)
	if err != nil {
		return err
	}

	_, err = w.client.UpdateGroupParticipants(context.Background(), chatJID, []types.JID{userJID}, whatsmeow.ParticipantChangeRemove)
	return err
}

func (w *WhatsAppAdapter) Mute(roomID, userID string, d time.Duration) error {
	return fmt.Errorf("whatsapp mute not supported via this adapter")
}

func (w *WhatsAppAdapter) DeleteMessage(msgID string) error {
	ctx := context.Background()
	msg, err := w.store.GetMessage(ctx, msgID)
	if err != nil {
		return err
	}

	chatJID, err := types.ParseJID(strings.TrimPrefix(msg.Room.ID, "whatsapp:"))
	if err != nil {
		return err
	}

	revokeMsg := w.client.BuildRevoke(chatJID, types.EmptyJID, msg.PlatformID)
	_, err = w.client.SendMessage(ctx, chatJID, revokeMsg)
	return err
}
