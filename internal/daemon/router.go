package daemon

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fossism/chaind-cli/internal/adapters"
	"github.com/fossism/chaind-cli/internal/schema"
	"github.com/fossism/chaind-cli/internal/store"
)

// AdapterRouter acts as the central registry dispatching messages from the IPC layer to the correct platform adapter.
type AdapterRouter struct {
	mu       sync.RWMutex
	adapters map[string]adapters.Adapter
	store    *store.Store
}

func NewAdapterRouter(st *store.Store) *AdapterRouter {
	return &AdapterRouter{
		adapters: make(map[string]adapters.Adapter),
		store:    st,
	}
}

func (r *AdapterRouter) Register(a adapters.Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.Platform()] = a
}

func (r *AdapterRouter) Unregister(platform string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.adapters, platform)
}

func (r *AdapterRouter) Get(platform string) (adapters.Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	if a, exists := r.adapters[platform]; exists {
		return a, nil
	}
	return nil, fmt.Errorf("adapter %s is not active or not registered", platform)
}

// Global Send Coordinator 
func (r *AdapterRouter) Send(platform, roomID, text string) (schema.Message, error) {
	adp, err := r.Get(platform)
	if err != nil {
		return schema.Message{}, err
	}
	msg, err := adp.Send(roomID, text)
	if err == nil {
		r.store.PushMessage(msg)
	}
	return msg, err
}

// Global Reply Coordinator
func (r *AdapterRouter) Reply(platform, msgID, text string) (schema.Message, error) {
	adp, err := r.Get(platform)
	if err != nil {
		return schema.Message{}, err
	}
	msg, err := adp.Reply(msgID, text)
	if err == nil {
		r.store.PushMessage(msg)
	}
	return msg, err
}

// Global React Coordinator
func (r *AdapterRouter) React(platform, msgID, emoji string) error {
	adp, err := r.Get(platform)
	if err != nil {
		return err
	}
	return adp.React(msgID, emoji)
}

// Global Delete Coordinator
func (r *AdapterRouter) DeleteMessage(platform, msgID string) error {
	adp, err := r.Get(platform)
	if err != nil {
		return err
	}
	return adp.DeleteMessage(msgID)
}

// Global Moderate Coordinator
func (r *AdapterRouter) Ban(platform, roomID, userID, reason string) error {
	adp, err := r.Get(platform)
	if err != nil {
		return err
	}
	return adp.Ban(roomID, userID, reason)
}

func (r *AdapterRouter) Mute(platform, roomID, userID string, duration time.Duration) error {
	adp, err := r.Get(platform)
	if err != nil {
		return err
	}
	return adp.Mute(roomID, userID, duration)
}

// WatchAll multiplexes watch streams from all registered adapters into a single channel.
func (r *AdapterRouter) WatchAll(ctx context.Context) (<-chan schema.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(chan schema.Message, 100)
	var wg sync.WaitGroup

	for _, adp := range r.adapters {
		wg.Add(1)
		go func(a adapters.Adapter) {
			defer wg.Done()
			ch, err := a.Watch(ctx, "")
			if err != nil {
				return
			}
			for msg := range ch {
				select {
				case out <- msg:
				case <-ctx.Done():
					return
				}
			}
		}(adp)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out, nil
}

