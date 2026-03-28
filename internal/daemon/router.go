package daemon

import (
	"fmt"
	"sync"
	"time"

	"github.com/fossism/chaind-cli/internal/adapters"
	"github.com/fossism/chaind-cli/internal/schema"
)

// AdapterRouter acts as the central registry dispatching messages from the IPC layer to the correct platform adapter.
type AdapterRouter struct {
	mu       sync.RWMutex
	adapters map[string]adapters.Adapter
}

func NewAdapterRouter() *AdapterRouter {
	return &AdapterRouter{
		adapters: make(map[string]adapters.Adapter),
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
	// Normalization and validation could occur here before handing off to the specific adapter
	return adp.Send(roomID, text)
}

// Global Reply Coordinator
func (r *AdapterRouter) Reply(platform, msgID, text string) (schema.Message, error) {
	adp, err := r.Get(platform)
	if err != nil {
		return schema.Message{}, err
	}
	return adp.Reply(msgID, text)
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
