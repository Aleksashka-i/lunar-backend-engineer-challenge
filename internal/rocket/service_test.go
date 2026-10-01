package rocket_test

import (
	"context"
	"math/rand/v2"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/rocket"
	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/storage"
)

func openService(t *testing.T, path string) (*rocket.Service, *storage.SQLite) {
	t.Helper()
	db, err := storage.OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return rocket.NewService(db), db
}

func newService(t *testing.T) (*rocket.Service, *storage.SQLite) {
	return openService(t, filepath.Join(t.TempDir(), "rockets.db"))
}

func process(t *testing.T, svc *rocket.Service, msgs ...rocket.Message) {
	t.Helper()
	for _, m := range msgs {
		require.NoError(t, svc.ProcessMessage(context.Background(), m))
	}
}

func get(t *testing.T, svc *rocket.Service, channel string) rocket.Rocket {
	t.Helper()
	r, err := svc.Get(context.Background(), channel)
	require.NoError(t, err)
	return r
}

func pendingNumbers(t *testing.T, db *storage.SQLite, channel string) []int {
	t.Helper()
	var numbers []int
	require.NoError(t, db.WithTx(context.Background(), func(tx rocket.TxStore) error {
		pending, err := tx.GetPendingMessages(context.Background(), channel)
		for _, m := range pending {
			numbers = append(numbers, m.Metadata.MessageNumber)
		}
		return err
	}))
	return numbers
}

func TestProcessMessageRejectsInvalidMessages(t *testing.T) {
	svc, _ := newService(t)
	tests := map[string]rocket.Message{
		"missing channel":     msg("", 1, rocket.TypeSpeedIncreased, rocket.SpeedIncreased{By: 1}),
		"zero message number": msg("ch1", 0, rocket.TypeSpeedIncreased, rocket.SpeedIncreased{By: 1}),
		"missing event":       msg("ch1", 1, "", nil),
	}
	for name, m := range tests {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, svc.ProcessMessage(context.Background(), m), rocket.ErrInvalidMessage)
		})
	}
}

func TestProcessMessageInOrder(t *testing.T) {
	svc, _ := newService(t)
	process(t, svc, flight("ch1")...)
	assert.Equal(t, flightState(), get(t, svc, "ch1"))

	_, err := svc.Get(context.Background(), "unknown")
	assert.ErrorIs(t, err, rocket.ErrNotFound)
}

func TestProcessMessageEarlyCreatesRocketAwaitingLaunch(t *testing.T) {
	svc, db := newService(t)
	process(t, svc, flight("ch1")[1])

	r := get(t, svc, "ch1")
	assert.Equal(t, rocket.StatusAwaitingLaunch, r.Status)
	assert.Equal(t, 0, r.LastMessageNumber)
	assert.Equal(t, []int{2}, pendingNumbers(t, db, "ch1"))
}

// TestProcessMessagePendingRows checks that pending_messages holds exactly the
// messages still waiting: early ones are stored and applied ones deleted.
func TestProcessMessagePendingRows(t *testing.T) {
	msgs := flight("ch1")
	svc, db := newService(t)

	process(t, svc, msgs[2], msgs[3], msgs[3]) // 3, 4, then 4 again
	assert.Equal(t, []int{3, 4}, pendingNumbers(t, db, "ch1"))
	process(t, svc, msgs[0]) // applied at once; 3 and 4 still wait for 2
	assert.Equal(t, []int{3, 4}, pendingNumbers(t, db, "ch1"))
	process(t, svc, msgs[1]) // fills the gap: 2, 3 and 4 applied
	assert.Empty(t, pendingNumbers(t, db, "ch1"))
	process(t, svc, msgs[1]) // repeat of an applied message
	assert.Empty(t, pendingNumbers(t, db, "ch1"))

	assert.Equal(t, flightState(), get(t, svc, "ch1"))
}

func TestProcessMessageOutOfOrderWithDuplicates(t *testing.T) {
	for range 20 {
		svc, db := newService(t)
		msgs := append(flight("ch1"), flight("ch1")...)
		rand.Shuffle(len(msgs), func(i, j int) { msgs[i], msgs[j] = msgs[j], msgs[i] })
		process(t, svc, msgs...)

		assert.Equal(t, flightState(), get(t, svc, "ch1"))
		assert.Empty(t, pendingNumbers(t, db, "ch1"))
	}
}

// TestProcessMessageSurvivesRestart checks that a message waiting for an earlier one
// survives a restart and is applied once the earlier one arrives.
func TestProcessMessageSurvivesRestart(t *testing.T) {
	msgs := flight("ch1")
	path := filepath.Join(t.TempDir(), "rockets.db")

	svc, db := openService(t, path)
	process(t, svc, msgs[1], msgs[2], msgs[3])
	require.NoError(t, db.Close())

	svc, _ = openService(t, path)
	process(t, svc, msgs[0])
	assert.Equal(t, flightState(), get(t, svc, "ch1"))
}

func TestListSorted(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	process(t, svc,
		msg("a", 1, rocket.TypeLaunched, rocket.Launched{Type: "Falcon-9", LaunchSpeed: 300, Mission: "GEMINI"}),
		msg("b", 1, rocket.TypeLaunched, rocket.Launched{Type: "Atlas-H", LaunchSpeed: 100, Mission: "APOLLO"}),
		msg("c", 1, rocket.TypeLaunched, rocket.Launched{Type: "Atlas-H", LaunchSpeed: 200, Mission: "ARTEMIS"}),
	)

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
		rockets, err := svc.List(ctx, tt.field, tt.desc)
		require.NoError(t, err)

		var got []string
		for _, r := range rockets {
			got = append(got, r.Channel)
		}
		assert.Equal(t, tt.want, got, "List(%s, desc=%v)", tt.field, tt.desc)
	}
}

func TestListSortedByStatusAndMessages(t *testing.T) {
	ctx := context.Background()
	svc, _ := newService(t)
	process(t, svc,
		// a: exploded after 2 messages
		msg("a", 1, rocket.TypeLaunched, rocket.Launched{Type: "Falcon-9", LaunchSpeed: 300, Mission: "GEMINI"}),
		msg("a", 2, rocket.TypeExploded, rocket.Exploded{Reason: "PRESSURE_VESSEL_FAILURE"}),
		// b: launched, 3 messages
		msg("b", 1, rocket.TypeLaunched, rocket.Launched{Type: "Atlas-H", LaunchSpeed: 100, Mission: "APOLLO"}),
		msg("b", 2, rocket.TypeSpeedIncreased, rocket.SpeedIncreased{By: 1}),
		msg("b", 3, rocket.TypeSpeedIncreased, rocket.SpeedIncreased{By: 1}),
		// c: awaiting launch, no messages applied yet
		msg("c", 2, rocket.TypeSpeedIncreased, rocket.SpeedIncreased{By: 1}),
	)

	tests := []struct {
		field rocket.SortField
		desc  bool
		want  []string
	}{
		{rocket.SortByStatus, false, []string{"c", "b", "a"}}, // awaiting launch, launched, exploded
		{rocket.SortByStatus, true, []string{"a", "b", "c"}},
		{rocket.SortByLastMessageNumber, false, []string{"c", "a", "b"}},
		{rocket.SortByLastMessageNumber, true, []string{"b", "a", "c"}},
	}
	for _, tt := range tests {
		rockets, err := svc.List(ctx, tt.field, tt.desc)
		require.NoError(t, err)

		var got []string
		for _, r := range rockets {
			got = append(got, r.Channel)
		}
		assert.Equal(t, tt.want, got, "List(%s, desc=%v)", tt.field, tt.desc)
	}
}
