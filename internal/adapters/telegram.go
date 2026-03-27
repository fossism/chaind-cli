package adapters

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/fossism/chaind-cli/internal/store"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog/log"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/gotd/td/tg"
)

type TelegramAdapter struct {
	client   *gotgproto.Client
	store    *store.Store
	apiID    int
	apiHash  string
	token    string
	mu       sync.RWMutex
	watchers map[string][]chan schema.Message
}

func NewTelegramAdapter(st *store.Store, apiIDStr, apiHash, botToken string) (*TelegramAdapter, error) {
	apiID, err := strconv.Atoi(apiIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid api_id: must be integer")
	}

	return &TelegramAdapter{
		store:    st,
		apiID:    apiID,
		apiHash:  apiHash,
		token:    botToken,
		watchers: make(map[string][]chan schema.Message),
	}, nil
}

func (t *TelegramAdapter) Start(ctx context.Context) error {
	log.Info().Msg("Telegram MTProto connecting...")

	clientType := gotgproto.ClientTypeBot(t.token)
	
	client, err := gotgproto.NewClient(
		t.apiID,
		t.apiHash,
		clientType,
		&gotgproto.ClientOpts{
			Session: sessionMaker.SimpleSession(),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to init gotgproto: %w", err)
	}

	t.client = client

	errChan := make(chan error, 1)
	go func() {
		errChan <- client.Idle()
	}()

	select {
	case <-ctx.Done():
		log.Info().Msg("Telegram sync loop stopping...")
		return nil
	case err := <-errChan:
		return fmt.Errorf("telegram client idle exit: %w", err)
	}
}

func (t *TelegramAdapter) handleMessage(msg tg.MessageClass) error {
	switch m := msg.(type) {
	case *tg.Message:
		text := m.Message
		if text == "" {
			return nil
		}

		senderID := "unknown"
		if peer, ok := m.GetPeerID().(*tg.PeerUser); ok {
			senderID = strconv.FormatInt(peer.UserID, 10)
		} else if peer, ok := m.GetPeerID().(*tg.PeerChannel); ok {
			senderID = strconv.FormatInt(peer.ChannelID, 10)
		} else if peer, ok := m.GetPeerID().(*tg.PeerChat); ok {
			senderID = strconv.FormatInt(peer.ChatID, 10)
		}
		
		roomID := senderID // In MTProto DMs, room is the sender's Peer ID
		
		msgOut := schema.Message{
			SchemaVersion: "1.0",
			ID:         ulid.Make().String(),
			Platform:   "telegram",
			PlatformID: strconv.Itoa(m.ID),
			Room: schema.Room{
				ID: fmt.Sprintf("telegram:%s", roomID),
			},
			Author: schema.Author{
				ID: senderID,
			},
			Content: schema.Content{
				Type: "text",
				Text: text,
			},
			Timestamp: time.Unix(int64(m.Date), 0).UTC(),
		}

		// Persist
		t.store.PushMessage(msgOut)

		// Broadcast
		t.mu.RLock()
		defer t.mu.RUnlock()
		for _, ch := range t.watchers[roomID] {
			select {
			case ch <- msgOut:
			default:
			}
		}
		for _, ch := range t.watchers[""] {
			select {
			case ch <- msgOut:
			default:
			}
		}

	case *tg.MessageEmpty:
		// Deleted
	}

	return nil
}

func (t *TelegramAdapter) Platform() string {
	return "telegram"
}

func (t *TelegramAdapter) Disconnect() error {
	return nil
}

func (t *TelegramAdapter) ReadHistory(roomID string, limit int, since time.Time) ([]schema.Message, error) {
	// Implementation calls to MessagesGetHistory would go here via proper tg.InputPeer setup.
	return nil, nil
}

func (t *TelegramAdapter) Watch(ctx context.Context, roomID string) (<-chan schema.Message, error) {
	ch := make(chan schema.Message, 100)
	
	t.mu.Lock()
	t.watchers[roomID] = append(t.watchers[roomID], ch)
	t.mu.Unlock()

	go func() {
		<-ctx.Done()
		t.mu.Lock()
		defer t.mu.Unlock()
		
		var updated []chan schema.Message
		for _, w := range t.watchers[roomID] {
			if w != ch {
				updated = append(updated, w)
			}
		}
		t.watchers[roomID] = updated
		close(ch)
	}()

	return ch, nil
}

func (t *TelegramAdapter) Send(roomID, text string) (schema.Message, error) {
	// Implementation calls to t.client.SendMessage(ctx, peer, opts) would go here
	// Mock MTProto SendMessage for architectural proof
	return schema.Message{
		ID: ulid.Make().String(),
		Platform: "telegram",
		PlatformID: "mocked_mtproto_id",
	}, nil
}

func (t *TelegramAdapter) Reply(msgID, text string) (schema.Message, error) {
	return schema.Message{}, fmt.Errorf("reply using ReplyToMsgId param on SendMessageOpts")
}

func (t *TelegramAdapter) React(msgID, emoji string) error {
	return fmt.Errorf("react using MessagesSendReactionRequest")
}

func (t *TelegramAdapter) Ban(roomID, userID, reason string) error {
	return fmt.Errorf("channelsEditBanned implementation")
}

func (t *TelegramAdapter) Mute(roomID, userID string, d time.Duration) error {
	return fmt.Errorf("channelsEditBanned implementation")
}

func (t *TelegramAdapter) DeleteMessage(msgID string) error {
	return fmt.Errorf("messagesDeleteMessages implementation")
}
