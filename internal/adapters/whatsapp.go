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
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

type WhatsAppAdapter struct {
	client      *whatsmeow.Client
	store       *store.Store
	mu          sync.RWMutex
	watchers    map[string][]chan schema.Message
	handlerOnce sync.Once
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

	w.handlerOnce.Do(func() {
		w.client.AddEventHandler(func(evt interface{}) {
			w.handleEvent(evt)
		})
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

// waWatchKey normalizes room filter keys so "whatsapp:<jid>", full JID,
// and legacy bare user parts all resolve to the same bucket.
func waWatchKey(room string) string {
	room = strings.TrimSpace(strings.TrimPrefix(room, "whatsapp:"))
	return room
}

// parseWAJID accepts "whatsapp:<jid>", full JID, or legacy bare user part.
func parseWAJID(s string) (types.JID, error) {
	s = strings.TrimSpace(strings.TrimPrefix(s, "whatsapp:"))
	if s == "" {
		return types.EmptyJID, fmt.Errorf("empty whatsapp jid")
	}
	if !strings.Contains(s, "@") {
		s = s + "@" + types.DefaultUserServer
	}
	return types.ParseJID(s)
}

func waBroadcastKeys(chat types.JID) []string {
	full := chat.String()
	keys := []string{waWatchKey(full)}
	if bare := strings.TrimSpace(chat.User); bare != "" && bare != full && waWatchKey(bare) != keys[0] {
		keys = append(keys, waWatchKey(bare))
	}
	return keys
}

// extractWAContent maps the common WhatsApp payloads to display text plus
// attachment metadata. Previously only conversation/image/document were
// covered, so voice notes, videos, stickers, locations, contacts, polls,
// and button/list replies were silently dropped ("not fetching").
func extractWAContent(m *waE2E.Message) (string, []schema.Attachment) {
	if m == nil {
		return "", nil
	}
	if s := m.GetConversation(); s != "" {
		return s, nil
	}
	if ext := m.GetExtendedTextMessage(); ext != nil {
		return ext.GetText(), nil
	}
	var atts []schema.Attachment
	if img := m.GetImageMessage(); img != nil {
		return img.GetCaption(), []schema.Attachment{{
			URI:      "whatsapp-image",
			MimeType: img.GetMimetype(),
			Size:     int64(img.GetFileLength()),
		}}
	}
	if doc := m.GetDocumentMessage(); doc != nil {
		return doc.GetCaption(), []schema.Attachment{{
			URI:      "whatsapp-document",
			MimeType: doc.GetMimetype(),
			Size:     int64(doc.GetFileLength()),
			Filename: doc.GetTitle(),
		}}
	}
	if vid := m.GetVideoMessage(); vid != nil {
		caption := vid.GetCaption()
		if vid.GetGifPlayback() && caption == "" {
			caption = "[gif]"
		}
		return caption, []schema.Attachment{{
			URI:      "whatsapp-video",
			MimeType: vid.GetMimetype(),
			Size:     int64(vid.GetFileLength()),
		}}
	}
	if aud := m.GetAudioMessage(); aud != nil {
		label := "[audio]"
		if aud.GetPTT() {
			label = "[voice note]"
		}
		return label, []schema.Attachment{{
			URI:      "whatsapp-audio",
			MimeType: aud.GetMimetype(),
			Size:     int64(aud.GetFileLength()),
		}}
	}
	if st := m.GetStickerMessage(); st != nil {
		return "[sticker]", []schema.Attachment{{
			URI:      "whatsapp-sticker",
			MimeType: st.GetMimetype(),
			Size:     int64(st.GetFileLength()),
		}}
	}
	if loc := m.GetLocationMessage(); loc != nil {
		name := loc.GetName()
		if name == "" {
			name = loc.GetAddress()
		}
		text := fmt.Sprintf("[location %.5f,%.5f %s]", loc.GetDegreesLatitude(), loc.GetDegreesLongitude(), strings.TrimSpace(name))
		return strings.TrimSpace(text), nil
	}
	if loc := m.GetLiveLocationMessage(); loc != nil {
		text := fmt.Sprintf("[live location %.5f,%.5f %s]", loc.GetDegreesLatitude(), loc.GetDegreesLongitude(), strings.TrimSpace(loc.GetCaption()))
		return strings.TrimSpace(text), nil
	}
	if c := m.GetContactMessage(); c != nil {
		name := c.GetDisplayName()
		if name == "" {
			name = "contact"
		}
		return fmt.Sprintf("[contact %s]", name), nil
	}
	if poll := m.GetPollCreationMessage(); poll != nil {
		var opts []string
		for _, o := range poll.GetOptions() {
			if o == nil {
				continue
			}
			if n := o.GetOptionName(); n != "" {
				opts = append(opts, n)
			}
		}
		text := "[poll " + strings.TrimSpace(poll.GetName())
		if len(opts) > 0 {
			text += ": " + strings.Join(opts, ", ")
		}
		return text + "]", nil
	}
	if br := m.GetButtonsResponseMessage(); br != nil {
		if t := br.GetSelectedDisplayText(); t != "" {
			return t, nil
		}
		return "[button " + br.GetSelectedButtonID() + "]", nil
	}
	if lr := m.GetListResponseMessage(); lr != nil {
		if t := lr.GetTitle(); t != "" {
			return t, nil
		}
		if d := lr.GetDescription(); d != "" {
			return d, nil
		}
		return "[list reply]", nil
	}
	_ = atts
	return "", nil
}

func (w *WhatsAppAdapter) handleEvent(rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.Message:
		// Protocol messages (history sync, revoke, edit shells) carry no
		// user content; the unwrapped evt.Message holds the real payload.
		if evt.Message.GetProtocolMessage() != nil {
			return
		}
		text, attachments := extractWAContent(evt.Message)

		if text == "" && len(attachments) == 0 {
			return
		}

		if evt.Info.Sender.IsEmpty() || evt.Info.Chat.IsEmpty() {
			log.Warn().Msg("Received WhatsApp message event with empty sender or chat info, skipping")
			return
		}

		// Preserve full JIDs (user@server) so DMs (s.whatsapp.net),
		// groups (g.us), channels, and LIDs stay distinct.
		chatJID := evt.Info.Chat
		senderJID := evt.Info.Sender
		if evt.Info.IsFromMe && senderJID.IsEmpty() {
			senderJID = chatJID
		}
		roomID := fmt.Sprintf("whatsapp:%s", chatJID.String())
		authorID := senderJID.String()

		msg := schema.Message{
			SchemaVersion: "1.0",
			ID:            ulid.Make().String(),
			Platform:      "whatsapp",
			PlatformID:    evt.Info.ID,
			Edited:        evt.IsEdit,
			Room:          schema.Room{ID: roomID},
			Author:        schema.Author{ID: authorID, DisplayName: evt.Info.PushName},
			Content:       schema.Content{Type: "text", Text: text, Attachments: attachments},
			Timestamp:     evt.Info.Timestamp.UTC(),
		}

		w.store.PushMessage(msg)

		w.mu.RLock()
		defer w.mu.RUnlock()
		for _, key := range waBroadcastKeys(chatJID) {
			for _, ch := range w.watchers[key] {
				select {
				case ch <- msg:
				default:
				}
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
	key := waWatchKey(roomID)

	w.mu.Lock()
	w.watchers[key] = append(w.watchers[key], ch)
	w.mu.Unlock()

	go func() {
		<-ctx.Done()
		w.mu.Lock()
		defer w.mu.Unlock()

		var updated []chan schema.Message
		for _, watchCh := range w.watchers[key] {
			if watchCh != ch {
				updated = append(updated, watchCh)
			}
		}
		w.watchers[key] = updated
		close(ch)
	}()

	return ch, nil
}

func (w *WhatsAppAdapter) Send(roomID, text string) (schema.Message, error) {
	if !w.client.IsConnected() {
		return schema.Message{}, fmt.Errorf("whatsapp client is not connected")
	}

	jid, err := parseWAJID(roomID)
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

	chatJID, err := parseWAJID(msg.Room.ID)
	if err != nil {
		return schema.Message{}, err
	}

	senderJID, err := parseWAJID(msg.Author.ID)
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

	chatJID, err := parseWAJID(msg.Room.ID)
	if err != nil {
		return err
	}

	senderJID, err := parseWAJID(msg.Author.ID)
	if err != nil {
		senderJID = chatJID
	}

	reactionMsg := w.client.BuildReaction(chatJID, senderJID, msg.PlatformID, emoji)
	_, err = w.client.SendMessage(ctx, chatJID, reactionMsg)
	return err
}

func (w *WhatsAppAdapter) Ban(roomID, userID, reason string) error {
	chatJID, err := parseWAJID(roomID)
	if err != nil {
		return err
	}
	userJID, err := parseWAJID(userID)
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

	chatJID, err := parseWAJID(msg.Room.ID)
	if err != nil {
		return err
	}

	revokeMsg := w.client.BuildRevoke(chatJID, types.EmptyJID, msg.PlatformID)
	_, err = w.client.SendMessage(ctx, chatJID, revokeMsg)
	return err
}
