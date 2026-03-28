package adapters

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time assertion that MockAdapter satisfies the Adapter interface.
var _ Adapter = (*MockAdapter)(nil)

func TestMockAdapter_Send_ReturnsULIDAndAppendsToOutbound(t *testing.T) {
	m := NewMockAdapter()
	msg, err := m.Send("room1", "hello")
	require.NoError(t, err)
	assert.NotEmpty(t, msg.ID)
	assert.Len(t, m.OutboundMessages, 1)
	assert.Equal(t, msg.ID, m.OutboundMessages[0].ID)
}

func TestMockAdapter_Send_MultipleCallsGrowSlice(t *testing.T) {
	m := NewMockAdapter()
	for i := 0; i < 5; i++ {
		_, err := m.Send("room1", "msg")
		require.NoError(t, err)
	}
	assert.Len(t, m.OutboundMessages, 5)
}

func TestMockAdapter_Disconnect_ReturnsNil(t *testing.T) {
	m := NewMockAdapter()
	assert.NoError(t, m.Disconnect())
}

func TestMockAdapter_React_ReturnsNil(t *testing.T) {
	m := NewMockAdapter()
	assert.NoError(t, m.React("msg1", "👍"))
}

func TestMockAdapter_Watch_ReturnsNonNilChannel(t *testing.T) {
	m := NewMockAdapter()
	ch, err := m.Watch(context.Background(), "room1")
	require.NoError(t, err)
	assert.NotNil(t, ch)
}

func TestMockAdapter_Platform(t *testing.T) {
	m := NewMockAdapter()
	assert.Equal(t, "mock", m.Platform())
}

func TestMockAdapter_ReadHistory_ReturnsEmpty(t *testing.T) {
	m := NewMockAdapter()
	msgs, err := m.ReadHistory("room1", 10, time.Now())
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

func TestMockAdapter_SendToWatch_DeliversToWatchChannel(t *testing.T) {
	m := NewMockAdapter()
	ch, err := m.Watch(context.Background(), "room1")
	require.NoError(t, err)

	sent, _ := m.Send("room1", "watch-test")
	m.SendToWatch(sent)

	select {
	case received := <-ch:
		assert.Equal(t, sent.ID, received.ID)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for watch message")
	}
}
