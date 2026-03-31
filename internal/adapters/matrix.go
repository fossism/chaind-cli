package adapters

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fossism/chaind-cli/internal/store"
	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog/log"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

type MatrixAdapter struct {
	client *mautrix.Client
	store  *store.Store
	mu       sync.RWMutex
	watchers map[string][]chan schema.Message
}

func NewMatrixAdapter(st *store.Store, homeServer string, userID string, accessToken string) (*MatrixAdapter, error) {
	client, err := mautrix.NewClient(homeServer, id.UserID(userID), accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create matrix client: %w", err)
	}

	// Hot-fix: If the user provided a token but no explicit UserID (or the fallback), resolve it!
	if userID == "" || userID == "@example:matrix.org" {
		who, err := client.Whoami(context.Background())
		if err == nil {
			log.Info().Str("matrix_user_id", string(who.UserID)).Msg("Auto-detected Matrix User ID successfully")
			client.UserID = who.UserID
		} else {
			log.Warn().Err(err).Msg("Failed to auto-detect Matrix User ID, token might be invalid or homeserver unreachable")
		}
	}

	return &MatrixAdapter{
		client:   client,
		store:    st,
		watchers: make(map[string][]chan schema.Message),
	}, nil
}

func (m *MatrixAdapter) Platform() string {
	return "matrix"
}

func (m *MatrixAdapter) Start(ctx context.Context) error {
	syncer := m.client.Syncer.(*mautrix.DefaultSyncer)
	
	// Load the stored sync token
	if token, err := m.store.GetSyncState(ctx, "matrix", "next_batch"); err == nil && token != "" {
		m.client.Store.SaveNextBatch(ctx, m.client.UserID, token)
	}

	syncer.OnEventType(event.EventMessage, func(_ context.Context, evt *event.Event) {
		m.handleMessage(evt)
	})

	syncer.OnSync(func(syncCtx context.Context, resp *mautrix.RespSync, since string) bool {
		m.store.SetSyncState(syncCtx, "matrix", "next_batch", resp.NextBatch)
		return true // Continue syncing
	})

	log.Info().Msg("Matrix sync loop starting...")

	errChan := make(chan error, 1)
	go func() {
		errChan <- m.client.Sync()
	}()

	select {
	case <-ctx.Done():
		log.Info().Msg("Matrix sync loop stopping...")
		m.client.StopSync()
		return nil
	case err := <-errChan:
		return fmt.Errorf("matrix sync failed: %w", err)
	}
}

func (m *MatrixAdapter) Disconnect() error {
	m.client.StopSync()
	return nil
}

func (m *MatrixAdapter) ReadHistory(roomID string, limit int, since time.Time) ([]schema.Message, error) {
	resp, err := m.client.Messages(context.Background(), id.RoomID(roomID), "", "", 'b', nil, limit)
	if err != nil {
		return nil, err
	}

	var msgs []schema.Message
	for _, evt := range resp.Chunk {
		if evt.Type == event.EventMessage {
			if content, ok := evt.Content.Parsed.(*event.MessageEventContent); ok {
				msgs = append(msgs, schema.Message{
					ID:         ulid.Make().String(),
					Platform:   "matrix",
					PlatformID: string(evt.ID),
					Room:       schema.Room{ID: fmt.Sprintf("matrix:%s", evt.RoomID)},
					Author:     schema.Author{ID: string(evt.Sender)},
					Content:    schema.Content{Type: "text", Text: content.Body},
					Timestamp:  time.UnixMilli(evt.Timestamp).UTC(),
				})
			}
		}
	}
	return msgs, nil
}

func (m *MatrixAdapter) Watch(ctx context.Context, roomID string) (<-chan schema.Message, error) {
	ch := make(chan schema.Message, 100)
	
	m.mu.Lock()
	m.watchers[roomID] = append(m.watchers[roomID], ch)
	m.mu.Unlock()

	go func() {
		<-ctx.Done()
		m.mu.Lock()
		defer m.mu.Unlock()
		
		var updated []chan schema.Message
		for _, w := range m.watchers[roomID] {
			if w != ch {
				updated = append(updated, w)
			}
		}
		m.watchers[roomID] = updated
		close(ch)
	}()

	return ch, nil
}

func (m *MatrixAdapter) Send(roomID, text string) (schema.Message, error) {
	targetRoom := strings.TrimPrefix(roomID, "matrix:")
	
	// Handle sending to a user ID by creating/finding a DM
	if strings.HasPrefix(targetRoom, "@") {
		resp, err := m.client.CreateRoom(context.Background(), &mautrix.ReqCreateRoom{
			Invite: []id.UserID{id.UserID(targetRoom)},
			IsDirect: true,
		})
		if err != nil {
			return schema.Message{}, fmt.Errorf("failed to create DM room with %s: %w", targetRoom, err)
		}
		targetRoom = string(resp.RoomID)
	}

	resp, err := m.client.SendText(context.Background(), id.RoomID(targetRoom), text)
	if err != nil {
		return schema.Message{}, err
	}
	return schema.Message{
		ID: ulid.Make().String(),
		Platform: "matrix",
		PlatformID: string(resp.EventID),
		Room: schema.Room{ID: "matrix:" + targetRoom},
		Content: schema.Content{Type: "text", Text: text},
		Timestamp: time.Now().UTC(),
	}, nil
}

func (m *MatrixAdapter) Reply(msgID, text string) (schema.Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	origMsg, err := m.store.GetMessage(ctx, msgID)
	if err != nil {
		return schema.Message{}, fmt.Errorf("failed to find original message for reply: %w", err)
	}
	
	roomIDStr := origMsg.Room.ID
	if len(roomIDStr) > 7 && roomIDStr[:7] == "matrix:" {
		roomIDStr = roomIDStr[7:]
	}

	content := event.MessageEventContent{
		MsgType: event.MsgText,
		Body:    text,
		RelatesTo: &event.RelatesTo{
			InReplyTo: &event.InReplyTo{
				EventID: id.EventID(origMsg.PlatformID),
			},
		},
	}

	resp, err := m.client.SendMessageEvent(context.Background(), id.RoomID(roomIDStr), event.EventMessage, content)
	if err != nil {
		return schema.Message{}, err
	}
	return schema.Message{
		ID:         ulid.Make().String(),
		Platform:   "matrix",
		PlatformID: string(resp.EventID),
		ParentID:   &origMsg.ID,
		Room:       schema.Room{ID: origMsg.Room.ID},
		Author:     schema.Author{ID: "self"},
		Content:    schema.Content{Type: "text", Text: text},
		Timestamp:  time.Now().UTC(),
	}, nil
}

func (m *MatrixAdapter) React(msgID, emoji string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	origMsg, err := m.store.GetMessage(ctx, msgID)
	if err != nil {
		return fmt.Errorf("failed to find original message: %w", err)
	}

	roomIDStr := origMsg.Room.ID
	if len(roomIDStr) > 7 && roomIDStr[:7] == "matrix:" {
		roomIDStr = roomIDStr[7:]
	}
	_, err = m.client.SendReaction(context.Background(), id.RoomID(roomIDStr), id.EventID(origMsg.PlatformID), emoji)
	return err
}

func (m *MatrixAdapter) Ban(roomID, userID, reason string) error {
	// Strip "matrix:" prefix if present
	if len(roomID) > 7 && roomID[:7] == "matrix:" {
		roomID = roomID[7:]
	}
	_, err := m.client.BanUser(context.Background(), id.RoomID(roomID), &mautrix.ReqBanUser{
		Reason: reason,
		UserID: id.UserID(userID),
	})
	return err
}

func (m *MatrixAdapter) Mute(roomID, userID string, d time.Duration) error {
	// Standard Matrix uses Power Levels to mute users.
	return nil
}

func (m *MatrixAdapter) DeleteMessage(msgID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	origMsg, err := m.store.GetMessage(ctx, msgID)
	if err != nil {
		return fmt.Errorf("failed to find message for delete: %w", err)
	}

	roomIDStr := origMsg.Room.ID
	if len(roomIDStr) > 7 && roomIDStr[:7] == "matrix:" {
		roomIDStr = roomIDStr[7:]
	}
	
	_, err = m.client.RedactEvent(context.Background(), id.RoomID(roomIDStr), id.EventID(origMsg.PlatformID), mautrix.ReqRedact{Reason: "deleted via chaind"})
	return err
}

func (m *MatrixAdapter) handleMessage(evt *event.Event) {
	msgContent, ok := evt.Content.Parsed.(*event.MessageEventContent)
	if !ok {
		return
	}

	log.Debug().
		Str("room", string(evt.RoomID)).
		Str("sender", string(evt.Sender)).
		Str("body", msgContent.Body).
		Msg("Matrix message received")

	msg := schema.Message{
		SchemaVersion: "1.0",
		ID:         ulid.Make().String(),
		Platform:   "matrix",
		PlatformID: string(evt.ID),
		Room: schema.Room{
			ID: fmt.Sprintf("matrix:%s", evt.RoomID),
		},
		Author: schema.Author{
			ID: string(evt.Sender),
		},
		Content: schema.Content{
			Type: "text",
			Text: msgContent.Body,
		},
		Timestamp:  time.UnixMilli(evt.Timestamp).UTC(),
	}

	// Persist
	m.store.PushMessage(msg)

	// Stream
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ch := range m.watchers[string(evt.RoomID)] {
		select {
		case ch <- msg:
		default:
		}
	}
	
	// Global listeners (empty string means watch all)
	for _, ch := range m.watchers[""] {
		select {
		case ch <- msg:
		default:
		}
	}
}
