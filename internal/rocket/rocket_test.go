package rocket_test

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/rocket"
)

var start = time.Date(2022, 2, 2, 19, 39, 5, 0, time.UTC)

func msg(channel string, number int, e rocket.Event) rocket.Message {
	return rocket.Message{
		Metadata: rocket.Metadata{
			Channel:       channel,
			MessageNumber: number,
			MessageTime:   start.Add(time.Duration(number) * time.Second),
		},
		Event: e,
	}
}

// flight is an in-order message stream for one rocket; flightState is its end state.
func flight(channel string) []rocket.Message {
	return []rocket.Message{
		msg(channel, 1, rocket.Launched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"}),
		msg(channel, 2, rocket.SpeedIncreased{By: 3000}),
		msg(channel, 3, rocket.SpeedDecreased{By: 1000}),
		msg(channel, 4, rocket.MissionChanged{NewMission: "SHUTTLE_MIR"}),
	}
}

func flightState() rocket.State {
	return rocket.State{
		Channel:           "ch1",
		Status:            rocket.StatusLaunched,
		Type:              "Falcon-9",
		Mission:           "SHUTTLE_MIR",
		Speed:             2500,
		LastMessageNumber: 4,
		LastMessageTime:   start.Add(4 * time.Second),
	}
}

func receive(r *rocket.Rocket, msgs ...rocket.Message) {
	for _, m := range msgs {
		r.Receive(m)
	}
}

func TestReceiveInOrder(t *testing.T) {
	r := rocket.NewRocket("ch1")
	receive(r, flight("ch1")...)
	assert.Equal(t, flightState(), r.State)
}

func TestReceiveOutOfOrderWithDuplicates(t *testing.T) {
	for range 100 {
		msgs := append(flight("ch1"), flight("ch1")...)
		rand.Shuffle(len(msgs), func(i, j int) { msgs[i], msgs[j] = msgs[j], msgs[i] })

		r := rocket.NewRocket("ch1")
		receive(r, msgs...)
		assert.Equal(t, flightState(), r.State)
		assert.Empty(t, r.Pending)
	}
}

func TestReceiveWaitsForMissingMessage(t *testing.T) {
	msgs := flight("ch1")
	r := rocket.NewRocket("ch1")

	receive(r, msgs[0], msgs[2], msgs[3])
	require.Equal(t, 1, r.LastMessageNumber)
	require.Equal(t, 500, r.Speed)
	require.Len(t, r.Pending, 2)

	receive(r, msgs[1])
	assert.Equal(t, flightState(), r.State)
}

func TestReceiveExploded(t *testing.T) {
	r := rocket.NewRocket("ch1")
	receive(r,
		msg("ch1", 1, rocket.Launched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"}),
		msg("ch1", 2, rocket.Exploded{Reason: "PRESSURE_VESSEL_FAILURE"}),
	)
	assert.Equal(t, rocket.StatusExploded, r.Status)
	assert.Equal(t, "PRESSURE_VESSEL_FAILURE", r.ExplosionReason)
}

func TestReceiveAwaitingLaunch(t *testing.T) {
	msgs := flight("ch1")
	r := rocket.NewRocket("ch1")

	receive(r, msgs[1], msgs[2])
	require.Equal(t, rocket.StatusAwaitingLaunch, r.Status)

	receive(r, msgs[0])
	assert.Equal(t, rocket.StatusLaunched, r.Status)
}

func TestReceiveMessageStatus(t *testing.T) {
	msgs := flight("ch1")
	r := rocket.NewRocket("ch1")

	assert.Equal(t, rocket.MessagePending, r.Receive(msgs[1]), "message 2 before message 1")
	assert.Equal(t, rocket.MessageDuplicate, r.Receive(msgs[1]), "message 2 again while pending")
	assert.Equal(t, rocket.MessageApplied, r.Receive(msgs[0]), "message 1 fills the gap")
	assert.Equal(t, rocket.MessageDuplicate, r.Receive(msgs[1]), "message 2 again after applied")
	assert.Equal(t, rocket.MessageApplied, r.Receive(msgs[2]), "message 3 in order")
}
