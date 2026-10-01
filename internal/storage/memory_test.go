package storage_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/rocket"
	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/storage"
)

func TestMemoryGetUnknown(t *testing.T) {
	_, err := storage.NewMemory().Get(context.Background(), "unknown")
	assert.ErrorIs(t, err, rocket.ErrNotFound)
}

func TestMemoryUpdateCreatesRocket(t *testing.T) {
	ctx := context.Background()
	m := storage.NewMemory()
	require.NoError(t, m.Update(ctx, "ch1", func(r *rocket.Rocket) { r.Speed = 100 }))

	s, err := m.Get(ctx, "ch1")
	require.NoError(t, err)
	assert.Equal(t, "ch1", s.Channel)
	assert.Equal(t, rocket.StatusAwaitingLaunch, s.Status)
	assert.Equal(t, 100, s.Speed)
}

// TestMemoryConcurrentUpdates is meant to be run with -race.
func TestMemoryConcurrentUpdates(t *testing.T) {
	ctx := context.Background()
	m := storage.NewMemory()
	channels := []string{"a", "b", "c"}

	var wg sync.WaitGroup
	for range 100 {
		for _, ch := range channels {
			wg.Go(func() {
				assert.NoError(t, m.Update(ctx, ch, func(r *rocket.Rocket) { r.Speed++ }))
			})
			wg.Go(func() {
				_, err := m.List(ctx)
				assert.NoError(t, err)
			})
		}
	}
	wg.Wait()

	for _, ch := range channels {
		s, err := m.Get(ctx, ch)
		require.NoError(t, err)
		assert.Equal(t, 100, s.Speed, ch)
	}
}
