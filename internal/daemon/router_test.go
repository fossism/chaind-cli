package daemon

import (
	"sync"
	"testing"

	"github.com/fossism/chaind-cli/internal/adapters"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdapterRouter_RegisterAndGet(t *testing.T) {
	r := NewAdapterRouter()
	m := adapters.NewMockAdapter()
	r.Register(m)

	got, err := r.Get("mock")
	require.NoError(t, err)
	assert.Equal(t, m, got)
}

func TestAdapterRouter_UnregisterRemovesEntry(t *testing.T) {
	r := NewAdapterRouter()
	m := adapters.NewMockAdapter()
	r.Register(m)
	r.Unregister("mock")

	_, err := r.Get("mock")
	assert.Error(t, err)
}

func TestAdapterRouter_Send_DelegatesToAdapter(t *testing.T) {
	r := NewAdapterRouter()
	m := adapters.NewMockAdapter()
	r.Register(m)

	msg, err := r.Send("mock", "room1", "hello")
	require.NoError(t, err)
	assert.NotEmpty(t, msg.ID)
	assert.Len(t, m.OutboundMessages, 1)
}

func TestAdapterRouter_Send_UnregisteredReturnsError(t *testing.T) {
	r := NewAdapterRouter()
	_, err := r.Send("nonexistent", "room1", "hello")
	assert.Error(t, err)
}

func TestAdapterRouter_Ban_DelegatesToAdapter(t *testing.T) {
	r := NewAdapterRouter()
	m := adapters.NewMockAdapter()
	r.Register(m)

	err := r.Ban("mock", "room1", "user1", "spam")
	assert.NoError(t, err)
}

func TestAdapterRouter_Concurrency(t *testing.T) {
	r := NewAdapterRouter()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m := adapters.NewMockAdapter()
			r.Register(m)
			_, _ = r.Get("mock")
			r.Unregister("mock")
		}()
	}
	wg.Wait()
}
