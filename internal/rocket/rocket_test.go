package rocket_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/rocket"
)

var start = time.Date(2022, 2, 2, 19, 39, 5, 0, time.UTC)

func msg(channel string, number int, messageType string, e rocket.Event) rocket.Message {
	return rocket.Message{
		Metadata: rocket.Metadata{
			Channel:       channel,
			MessageNumber: number,
			MessageTime:   start.Add(time.Duration(number) * time.Second),
			MessageType:   messageType,
		},
		Event: e,
	}
}

// flight is an in-order message stream for one rocket; flightState is its end state.
func flight(channel string) []rocket.Message {
	return []rocket.Message{
		msg(channel, 1, rocket.TypeLaunched, rocket.Launched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"}),
		msg(channel, 2, rocket.TypeSpeedIncreased, rocket.SpeedIncreased{By: 3000}),
		msg(channel, 3, rocket.TypeSpeedDecreased, rocket.SpeedDecreased{By: 1000}),
		msg(channel, 4, rocket.TypeMissionChanged, rocket.MissionChanged{NewMission: "SHUTTLE_MIR"}),
	}
}

func flightState() rocket.Rocket {
	return rocket.Rocket{
		Channel:           "ch1",
		Status:            rocket.StatusLaunched,
		Type:              "Falcon-9",
		Mission:           "SHUTTLE_MIR",
		Speed:             2500,
		LastMessageNumber: 4,
		LastMessageTime:   start.Add(4 * time.Second),
	}
}

func TestApplyInOrder(t *testing.T) {
	r := rocket.NewRocket("ch1")
	for _, m := range flight("ch1") {
		assert.Equal(t, rocket.MessageApplied, r.Apply(m))
	}
	assert.Equal(t, flightState(), r)
}

func TestApplyEarlyMessageChangesNothing(t *testing.T) {
	r := rocket.NewRocket("ch1")
	assert.Equal(t, rocket.MessagePending, r.Apply(flight("ch1")[1]))
	assert.Equal(t, rocket.NewRocket("ch1"), r)
}

func TestApplyRepeatChangesNothing(t *testing.T) {
	msgs := flight("ch1")
	r := rocket.NewRocket("ch1")
	r.Apply(msgs[0])
	r.Apply(msgs[1])
	before := r

	assert.Equal(t, rocket.MessageDuplicate, r.Apply(msgs[1]))
	assert.Equal(t, rocket.MessageDuplicate, r.Apply(msgs[0]))
	assert.Equal(t, before, r)
}

func TestApplyExploded(t *testing.T) {
	r := rocket.NewRocket("ch1")
	r.Apply(msg("ch1", 1, rocket.TypeLaunched, rocket.Launched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"}))
	r.Apply(msg("ch1", 2, rocket.TypeExploded, rocket.Exploded{Reason: "PRESSURE_VESSEL_FAILURE"}))
	assert.Equal(t, rocket.StatusExploded, r.Status)
	assert.Equal(t, "PRESSURE_VESSEL_FAILURE", r.ExplosionReason)
}

func TestApplyPendingStopsAtGap(t *testing.T) {
	msgs := flight("ch1")
	r := rocket.NewRocket("ch1")
	r.Apply(msgs[0])

	r.ApplyPending([]rocket.Message{msgs[1], msgs[3]}) // 3 is missing
	assert.Equal(t, 2, r.LastMessageNumber)
}

func TestApplyPendingSkipsApplied(t *testing.T) {
	msgs := flight("ch1")
	r := rocket.NewRocket("ch1")
	r.Apply(msgs[0])
	r.Apply(msgs[1])

	r.ApplyPending(msgs) // 1 and 2 are already applied
	assert.Equal(t, flightState(), r)
}
