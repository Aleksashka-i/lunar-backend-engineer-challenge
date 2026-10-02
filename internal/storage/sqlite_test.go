package storage_test

import (
	"context"
	"math/rand/v2"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"lunar-backend-engineer-challenge/internal/rocket"
	"lunar-backend-engineer-challenge/internal/storage"
)

// openSQLite opens the database at path and closes it when the test ends.
func openSQLite(t *testing.T, path string) *storage.SQLite {
	t.Helper()
	db, err := storage.OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func newMessage(channel string, number int64, messageType rocket.MessageType, e rocket.Event) rocket.Message {
	return rocket.Message{
		Metadata: rocket.Metadata{Channel: channel, MessageNumber: number, MessageType: messageType},
		Event:    e,
	}
}

func launch(channel string) rocket.Message {
	return newMessage(channel, 1, rocket.TypeLaunched, rocket.Launched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"})
}

func speedUp(channel string, number int64, by int) rocket.Message {
	return newMessage(channel, number, rocket.TypeSpeedIncreased, rocket.SpeedIncreased{By: by})
}

func TestGetRocketUnknown(t *testing.T) {
	db := openSQLite(t, filepath.Join(t.TempDir(), "rockets.db"))

	_, err := db.GetRocket(context.Background(), "unknown")
	assert.ErrorIs(t, err, rocket.ErrNotFound)
}

func TestProcessMessageSavesRocket(t *testing.T) {
	ctx := context.Background()
	db := openSQLite(t, filepath.Join(t.TempDir(), "rockets.db"))

	status, err := db.ProcessMessage(ctx, launch("ch1"), (*rocket.Rocket).Apply)
	require.NoError(t, err)
	assert.Equal(t, rocket.MessageApplied, status)
	status, err = db.ProcessMessage(ctx, speedUp("ch1", 2, 100), (*rocket.Rocket).Apply)
	require.NoError(t, err)
	assert.Equal(t, rocket.MessageApplied, status)

	r, err := db.GetRocket(ctx, "ch1")
	require.NoError(t, err)
	assert.Equal(t, rocket.StatusLaunched, r.Status)
	assert.Equal(t, 600, r.Speed)
	assert.Equal(t, int64(2), r.LastMessageNumber)

	rockets, err := db.ListRockets(ctx)
	require.NoError(t, err)
	assert.Equal(t, []rocket.Rocket{r}, rockets)
}

func TestProcessMessageAppliesPendingWhenGapFills(t *testing.T) {
	ctx := context.Background()
	db := openSQLite(t, filepath.Join(t.TempDir(), "rockets.db"))

	// message #3
	status, err := db.ProcessMessage(ctx, speedUp("ch1", 3, 10), (*rocket.Rocket).Apply)
	require.NoError(t, err)
	assert.Equal(t, rocket.MessagePending, status)
	r, err := db.GetRocket(ctx, "ch1")
	require.NoError(t, err)
	assert.Equal(t, rocket.Rocket{Channel: "ch1", Status: rocket.StatusAwaitingLaunch, PendingMessages: 1}, r, "an early message creates a rocket awaiting launch")
	// message #4
	status, err = db.ProcessMessage(ctx, speedUp("ch1", 4, 1), (*rocket.Rocket).Apply)
	require.NoError(t, err)
	assert.Equal(t, rocket.MessagePending, status)
	// message #1
	status, err = db.ProcessMessage(ctx, launch("ch1"), (*rocket.Rocket).Apply)
	require.NoError(t, err)
	assert.Equal(t, rocket.MessageApplied, status)

	r, err = db.GetRocket(ctx, "ch1")
	require.NoError(t, err)
	assert.Equal(t, int64(1), r.LastMessageNumber, "3 and 4 still wait for 2")
	assert.Equal(t, 2, r.PendingMessages)

	// message #2
	status, err = db.ProcessMessage(ctx, speedUp("ch1", 2, 100), (*rocket.Rocket).Apply)
	require.NoError(t, err)
	assert.Equal(t, rocket.MessageApplied, status)
	r, err = db.GetRocket(ctx, "ch1")
	require.NoError(t, err)
	assert.Equal(t, int64(4), r.LastMessageNumber)
	assert.Equal(t, 611, r.Speed)
	assert.Equal(t, 0, r.PendingMessages)

	// message #4 (duplicate)
	status, err = db.ProcessMessage(ctx, speedUp("ch1", 4, 1), (*rocket.Rocket).Apply)
	require.NoError(t, err)
	assert.Equal(t, rocket.MessageDuplicate, status)
	after, err := db.GetRocket(ctx, "ch1")
	require.NoError(t, err)
	assert.Equal(t, r, after, "a repeat changes nothing")
}

func TestProcessMessageKeepsFirstPending(t *testing.T) {
	ctx := context.Background()
	db := openSQLite(t, filepath.Join(t.TempDir(), "rockets.db"))

	// message #2
	_, err := db.ProcessMessage(ctx, speedUp("ch1", 2, 100), (*rocket.Rocket).Apply)
	require.NoError(t, err)
	// message #2 (duplicate)
	status, err := db.ProcessMessage(ctx, speedUp("ch1", 2, 999), (*rocket.Rocket).Apply)
	require.NoError(t, err)
	assert.Equal(t, rocket.MessagePending, status, "same number, different content")
	// message #1
	_, err = db.ProcessMessage(ctx, launch("ch1"), (*rocket.Rocket).Apply)
	require.NoError(t, err)

	r, err := db.GetRocket(ctx, "ch1")
	require.NoError(t, err)
	assert.Equal(t, 600, r.Speed, "the first stored message 2 is applied")
}

func TestSurvivesRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rockets.db")

	db := openSQLite(t, path)
	// message #2
	_, err := db.ProcessMessage(ctx, speedUp("ch1", 2, 100), (*rocket.Rocket).Apply)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	db = openSQLite(t, path)
	// message #1
	_, err = db.ProcessMessage(ctx, launch("ch1"), (*rocket.Rocket).Apply)
	require.NoError(t, err)
	r, err := db.GetRocket(ctx, "ch1")
	require.NoError(t, err)
	assert.Equal(t, 600, r.Speed)
}

func TestReset(t *testing.T) {
	ctx := context.Background()
	db := openSQLite(t, filepath.Join(t.TempDir(), "rockets.db"))
	// message #2
	_, err := db.ProcessMessage(ctx, speedUp("ch1", 2, 100), (*rocket.Rocket).Apply)
	require.NoError(t, err)

	require.NoError(t, db.Reset(ctx))
	_, err = db.GetRocket(ctx, "ch1")
	assert.ErrorIs(t, err, rocket.ErrNotFound)

	// message #1
	_, err = db.ProcessMessage(ctx, launch("ch1"), (*rocket.Rocket).Apply)
	require.NoError(t, err)
	r, err := db.GetRocket(ctx, "ch1")
	require.NoError(t, err)
	assert.Equal(t, 500, r.Speed, "the deleted pending message is not applied")
}

// TestConcurrentProcessMessages verifies the Store locking contract with the real domain rule:
// after launch, messages 2 to 101 for each rocket arrive concurrently, in random order and twice each,
// and none may be lost or applied twice. SQLite satisfies it through its single writer. Run with -race.
func TestConcurrentProcessMessages(t *testing.T) {
	ctx := context.Background()
	db := openSQLite(t, filepath.Join(t.TempDir(), "rockets.db"))
	channels := []string{"a", "b", "c"}
	for _, ch := range channels {
		_, err := db.ProcessMessage(ctx, launch(ch), (*rocket.Rocket).Apply)
		require.NoError(t, err)
	}

	var msgs []rocket.Message
	for n := int64(2); n <= 101; n++ {
		for _, ch := range channels {
			msgs = append(msgs, speedUp(ch, n, 1), speedUp(ch, n, 1)) // each delivered twice
		}
	}
	rand.Shuffle(len(msgs), func(i, j int) { msgs[i], msgs[j] = msgs[j], msgs[i] })

	var wg sync.WaitGroup
	for _, m := range msgs {
		wg.Go(func() {
			_, err := db.ProcessMessage(ctx, m, (*rocket.Rocket).Apply)
			assert.NoError(t, err)
		})
		wg.Go(func() {
			_, err := db.ListRockets(ctx)
			assert.NoError(t, err)
		})
	}
	wg.Wait()

	for _, ch := range channels {
		r, err := db.GetRocket(ctx, ch)
		require.NoError(t, err)
		assert.Equal(t, 600, r.Speed, ch)
		assert.Equal(t, int64(101), r.LastMessageNumber, ch)
	}
}
