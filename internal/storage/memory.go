// Package storage implements rocket.Repository.
package storage

import (
	"context"
	"sync"

	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/rocket"
)

// Memory is an in-memory rocket.Repository. State is lost on restart.
type Memory struct {
	mu      sync.RWMutex
	rockets map[string]*rocket.Rocket
}

var _ rocket.Repository = (*Memory)(nil)

// NewMemory creates an empty Memory.
func NewMemory() *Memory {
	return &Memory{rockets: make(map[string]*rocket.Rocket)}
}

// Update applies fn to the rocket for channel, creating it if needed.
func (m *Memory) Update(_ context.Context, channel string, fn func(*rocket.Rocket)) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.rockets[channel]
	if !ok {
		r = rocket.NewRocket(channel)
		m.rockets[channel] = r
	}
	fn(r)
	return nil
}

// Get returns the state of one rocket, or rocket.ErrNotFound.
func (m *Memory) Get(_ context.Context, channel string) (rocket.State, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	r, ok := m.rockets[channel]
	if !ok {
		return rocket.State{}, rocket.ErrNotFound
	}
	return r.State, nil
}

// List returns the states of all rockets in no particular order.
func (m *Memory) List(_ context.Context) ([]rocket.State, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make([]rocket.State, 0, len(m.rockets))
	for _, r := range m.rockets {
		states = append(states, r.State)
	}
	return states, nil
}
