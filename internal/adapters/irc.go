package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/fossism/chaind-cli/internal/store"
	"github.com/rs/zerolog/log"
)

type IRCAdapter struct {
	store  *store.Store
	server string
	nick   string
}

func NewIRCAdapter(st *store.Store, server, nick string) (*IRCAdapter, error) {
	return &IRCAdapter{
		store:  st,
		server: server,
		nick:   nick,
	}, nil
}

func (i *IRCAdapter) Platform() string {
	return "irc"
}

func (i *IRCAdapter) Start(ctx context.Context) error {
	log.Info().Str("server", i.server).Msg("IRC adapter placeholder started...")
	<-ctx.Done()
	return nil
}

func (i *IRCAdapter) Disconnect() error {
	return nil
}

func (i *IRCAdapter) ReadHistory(roomID string, limit int, since time.Time) ([]schema.Message, error) {
	return nil, fmt.Errorf("irc history not supported")
}

func (i *IRCAdapter) Watch(ctx context.Context, roomID string) (<-chan schema.Message, error) {
	return nil, fmt.Errorf("irc watch not implemented")
}

func (i *IRCAdapter) Send(roomID, text string) (schema.Message, error) {
	return schema.Message{}, fmt.Errorf("irc send not implemented")
}

func (i *IRCAdapter) Reply(msgID, text string) (schema.Message, error) {
	return schema.Message{}, fmt.Errorf("irc reply not supported")
}

func (i *IRCAdapter) React(msgID, emoji string) error {
	return fmt.Errorf("irc react not supported")
}

func (i *IRCAdapter) Ban(roomID, userID, reason string) error {
	return fmt.Errorf("irc ban not implemented")
}

func (i *IRCAdapter) Mute(roomID, userID string, d time.Duration) error {
	return fmt.Errorf("irc mute not implemented")
}

func (i *IRCAdapter) DeleteMessage(msgID string) error {
	return fmt.Errorf("irc message deletion not supported")
}
