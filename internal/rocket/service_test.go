package rocket_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/rocket"
	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/storage"
)

func TestIngestRejectsInvalidMessages(t *testing.T) {
	svc := rocket.NewService(storage.NewMemory())
	tests := map[string]rocket.Message{
		"missing channel":     msg("", 1, rocket.SpeedIncreased{By: 1}),
		"zero message number": msg("ch1", 0, rocket.SpeedIncreased{By: 1}),
		"missing event":       msg("ch1", 1, nil),
	}
	for name, m := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Ingest(context.Background(), m)
			assert.ErrorIs(t, err, rocket.ErrInvalidMessage)
		})
	}
}

func TestIngestAndGet(t *testing.T) {
	ctx := context.Background()
	svc := rocket.NewService(storage.NewMemory())
	for _, m := range flight("ch1") {
		_, err := svc.Ingest(ctx, m)
		require.NoError(t, err)
	}

	s, err := svc.Get(ctx, "ch1")
	require.NoError(t, err)
	assert.Equal(t, flightState(), s)

	_, err = svc.Get(ctx, "unknown")
	assert.ErrorIs(t, err, rocket.ErrNotFound)
}

func TestListSorted(t *testing.T) {
	ctx := context.Background()
	svc := rocket.NewService(storage.NewMemory())
	launches := []rocket.Message{
		msg("a", 1, rocket.Launched{Type: "Falcon-9", LaunchSpeed: 300, Mission: "GEMINI"}),
		msg("b", 1, rocket.Launched{Type: "Atlas-H", LaunchSpeed: 100, Mission: "APOLLO"}),
		msg("c", 1, rocket.Launched{Type: "Atlas-H", LaunchSpeed: 200, Mission: "ARTEMIS"}),
	}
	for _, m := range launches {
		_, err := svc.Ingest(ctx, m)
		require.NoError(t, err)
	}

	tests := []struct {
		field rocket.SortField
		desc  bool
		want  []string
	}{
		{rocket.SortByChannel, false, []string{"a", "b", "c"}},
		{rocket.SortByChannel, true, []string{"c", "b", "a"}},
		{rocket.SortBySpeed, false, []string{"b", "c", "a"}},
		{rocket.SortBySpeed, true, []string{"a", "c", "b"}},
		{rocket.SortByMission, false, []string{"b", "c", "a"}},
		{rocket.SortByType, false, []string{"b", "c", "a"}}, // ties broken by channel
	}
	for _, tt := range tests {
		states, err := svc.List(ctx, tt.field, tt.desc)
		require.NoError(t, err)

		var got []string
		for _, s := range states {
			got = append(got, s.Channel)
		}
		assert.Equal(t, tt.want, got, "List(%s, desc=%v)", tt.field, tt.desc)
	}
}
