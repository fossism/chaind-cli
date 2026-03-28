package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/fossism/chaind-cli/internal/store"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog/log"

	"github.com/celestix/gotgproto"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/dispatcher/handlers/filters"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/sessionMaker"
	"github.com/gotd/td/tg"
	"github.com/glebarez/sqlite"
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
	
	home, _ := os.UserHomeDir()
	sessionPath := filepath.Join(home, ".local", "share", "chaind", "telegram.session")

	client, err := gotgproto.NewClient(
		t.apiID,
		t.apiHash,
		clientType,
		&gotgproto.ClientOpts{
			Session: sessionMaker.SqlSession(sqlite.Open(sessionPath)),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to init gotgproto: %w", err)
	}

	t.client = client

	// Register message dispatcher
	client.Dispatcher.AddHandler(handlers.NewMessage(filters.Message.All, func(ctx *ext.Context, update *ext.Update) error {
		return t.handleMessage(update.EffectiveMessage)
	}))

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
		
		var attachments []schema.Attachment
		if m.Media != nil {
			switch media := m.Media.(type) {
			case *tg.MessageMediaPhoto:
				attachments = append(attachments, schema.Attachment{
					URI:      fmt.Sprintf("telegram-photo://%d", media.Photo.GetID()),
					MimeType: "image/jpeg",
					Size:     0,
				})
			case *tg.MessageMediaDocument:
				doc, ok := media.Document.AsNotEmpty()
				if ok {
					attachments = append(attachments, schema.Attachment{
						URI:      fmt.Sprintf("telegram-document://%d", doc.GetID()),
						MimeType: doc.MimeType,
						Size:     doc.Size,
					})
				}
			}
		}

		msgOut := schema.Message{
			SchemaVersion: "1.0",
			ID:            ulid.Make().String(),
			Platform:      "telegram",
			PlatformID:    strconv.Itoa(m.ID),
			Room: schema.Room{
				ID: fmt.Sprintf("telegram:%s", roomID),
			},
			Author: schema.Author{
				ID: senderID,
			},
			Content: schema.Content{
				Type:        "text",
				Text:        text,
				Attachments: attachments,
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

func parseChatID(roomID string) (int64, error) {
	// Strip "telegram:" prefix if present
	if len(roomID) > 9 && roomID[:9] == "telegram:" {
		roomID = roomID[9:]
	}
	return strconv.ParseInt(roomID, 10, 64)
}

func (t *TelegramAdapter) Send(roomID, text string) (schema.Message, error) {
	chatID, err := parseChatID(roomID)
	if err != nil {
		return schema.Message{}, fmt.Errorf("invalid telegram roomID: %w", err)
	}

	ctxExt := t.client.CreateContext()
	msgRaw, err := ctxExt.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: text,
	})
	if err != nil {
		return schema.Message{}, fmt.Errorf("failed to send message: %w", err)
	}

	platformID := "unknown"
	if msgRaw != nil {
		platformID = strconv.Itoa(msgRaw.ID)
	}

	return schema.Message{
		ID:         ulid.Make().String(),
		Platform:   "telegram",
		PlatformID: platformID,
		Room:       schema.Room{ID: fmt.Sprintf("telegram:%d", chatID)},
		Author:     schema.Author{ID: "self"}, // would normally be the bot/user ID
		Content:    schema.Content{Type: "text", Text: text},
		Timestamp:  time.Now().UTC(),
	}, nil
}

func (t *TelegramAdapter) Reply(msgID, text string) (schema.Message, error) {
	// Need to fetch original message to get room/chat ID and platform message ID
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	origMsg, err := t.store.GetMessage(ctx, msgID)
	if err != nil {
		return schema.Message{}, fmt.Errorf("failed to find original message for reply: %w", err)
	}

	chatID, err := parseChatID(origMsg.Room.ID)
	if err != nil {
		return schema.Message{}, fmt.Errorf("invalid telegram roomID: %w", err)
	}
	replyToID, err := strconv.Atoi(origMsg.PlatformID)
	if err != nil {
		return schema.Message{}, fmt.Errorf("invalid telegram original message ID: %w", err)
	}

	ctxExt := t.client.CreateContext()
	msgRaw, err := ctxExt.SendMessage(chatID, &tg.MessagesSendMessageRequest{
		Message: text,
		ReplyTo: &tg.InputReplyToMessage{ReplyToMsgID: replyToID},
	})
	if err != nil {
		return schema.Message{}, fmt.Errorf("failed to send reply: %w", err)
	}

	platformID := "unknown"
	if msgRaw != nil {
		platformID = strconv.Itoa(msgRaw.ID)
	}

	return schema.Message{
		ID:         ulid.Make().String(),
		Platform:   "telegram",
		PlatformID: platformID,
		Room:       schema.Room{ID: fmt.Sprintf("telegram:%d", chatID)},
		Author:     schema.Author{ID: "self"},
		Content:    schema.Content{Type: "text", Text: text},
		ParentID:   &origMsg.ID,
		Timestamp:  time.Now().UTC(),
	}, nil
}

func (t *TelegramAdapter) React(msgID, emoji string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	origMsg, err := t.store.GetMessage(ctx, msgID)
	if err != nil {
		return fmt.Errorf("failed to find original message for react: %w", err)
	}

	chatID, err := parseChatID(origMsg.Room.ID)
	if err != nil {
		return fmt.Errorf("invalid telegram roomID: %w", err)
	}
	replyToID, err := strconv.Atoi(origMsg.PlatformID)
	if err != nil {
		return fmt.Errorf("invalid telegram original message ID: %w", err)
	}

	ctxExt := t.client.CreateContext()
	inputPeer, err := ctxExt.ResolveInputPeerById(chatID)
	if err != nil {
		return fmt.Errorf("failed to resolve input peer: %w", err)
	}

	_, err = ctxExt.Raw.MessagesSendReaction(ctxExt, &tg.MessagesSendReactionRequest{
		Peer:  inputPeer,
		MsgID: replyToID,
		Reaction: []tg.ReactionClass{
			&tg.ReactionEmoji{Emoticon: emoji},
		},
	})
	return err
}

func (t *TelegramAdapter) Ban(roomID, userID, reason string) error {
	chatID, err := parseChatID(roomID)
	if err != nil {
		return fmt.Errorf("invalid telegram roomID: %w", err)
	}

	uID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram userID: %w", err)
	}

	ctxExt := t.client.CreateContext()
	// ban forever
	_, err = ctxExt.BanChatMember(chatID, uID, 0)
	if err != nil {
		return fmt.Errorf("failed to ban user %d in chat %d: %w", uID, chatID, err)
	}
	return nil
}

func (t *TelegramAdapter) Mute(roomID, userID string, d time.Duration) error {
	chatID, err := parseChatID(roomID)
	if err != nil {
		return fmt.Errorf("invalid telegram roomID: %w", err)
	}

	uID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid telegram userID: %w", err)
	}

	ctxExt := t.client.CreateContext()
	until := int(time.Now().Add(d).Unix())
	
	inputPeerGroup, err := ctxExt.ResolveInputPeerById(chatID)
	if err != nil {
		return err
	}
	inputChan, ok := inputPeerGroup.(*tg.InputPeerChannel)
	if !ok {
		return fmt.Errorf("muting is currently only supported in channels/supergroups (need input peer channel)")
	}
	
	inputPeerUser, err := ctxExt.ResolveInputPeerById(uID)
	if err != nil {
		return err
	}
	inputUser, ok := inputPeerUser.(*tg.InputPeerUser)
	if !ok {
		return fmt.Errorf("target user resolving failed")
	}

	// Mute via restrictions
	bannedRights := tg.ChatBannedRights{
		SendMessages: true,
		UntilDate:    until,
	}

	_, err = ctxExt.Raw.ChannelsEditBanned(ctxExt, &tg.ChannelsEditBannedRequest{
		Channel: &tg.InputChannel{ChannelID: inputChan.ChannelID, AccessHash: inputChan.AccessHash},
		Participant: &tg.InputPeerUser{UserID: inputUser.UserID, AccessHash: inputUser.AccessHash},
		BannedRights: bannedRights,
	})
	return err
}

func (t *TelegramAdapter) DeleteMessage(msgID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	origMsg, err := t.store.GetMessage(ctx, msgID)
	if err != nil {
		return fmt.Errorf("failed to find message for delete: %w", err)
	}

	chatID, err := parseChatID(origMsg.Room.ID)
	if err != nil {
		return fmt.Errorf("invalid telegram roomID: %w", err)
	}

	targetID, err := strconv.Atoi(origMsg.PlatformID)
	if err != nil {
		return fmt.Errorf("invalid telegram message ID for delete: %w", err)
	}

	ctxExt := t.client.CreateContext()
	return ctxExt.DeleteMessages(chatID, []int{targetID})
}
