package rocket_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"lunar-backend-engineer-challenge/internal/rocket"
)

func TestApplyLaunched(t *testing.T) {
	launchTime := time.Date(2022, 2, 2, 19, 39, 5, 0, time.UTC)
	r := rocket.NewRocket("ch1")

	status := r.Apply(rocket.Message{
		Metadata: rocket.Metadata{Channel: "ch1", MessageNumber: 1, MessageTime: launchTime, MessageType: rocket.TypeLaunched},
		Event:    rocket.Launched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"},
	})

	assert.Equal(t, rocket.MessageApplied, status)
	assert.Equal(t, rocket.Rocket{
		Channel:           "ch1",
		Status:            rocket.StatusLaunched,
		Type:              "Falcon-9",
		Mission:           "ARTEMIS",
		Speed:             500,
		LastMessageNumber: 1,
		LastMessageTime:   launchTime,
	}, r)
}

func TestApplySpeedIncreased(t *testing.T) {
	r := rocket.Rocket{
		Channel:           "ch1",
		Status:            rocket.StatusLaunched,
		Type:              "Falcon-9",
		Mission:           "ARTEMIS",
		Speed:             500,
		LastMessageNumber: 1,
	}

	status := r.Apply(rocket.Message{
		Metadata: rocket.Metadata{Channel: "ch1", MessageNumber: 2, MessageType: rocket.TypeSpeedIncreased},
		Event:    rocket.SpeedIncreased{By: 3000},
	})

	assert.Equal(t, rocket.MessageApplied, status)
	assert.Equal(t, rocket.Rocket{
		Channel:           "ch1",
		Status:            rocket.StatusLaunched,
		Type:              "Falcon-9",
		Mission:           "ARTEMIS",
		Speed:             3500,
		LastMessageNumber: 2,
	}, r)
}

func TestApplySpeedDecreased(t *testing.T) {
	r := rocket.Rocket{
		Channel:           "ch1",
		Status:            rocket.StatusLaunched,
		Type:              "Falcon-9",
		Mission:           "ARTEMIS",
		Speed:             3500,
		LastMessageNumber: 1,
	}

	status := r.Apply(rocket.Message{
		Metadata: rocket.Metadata{Channel: "ch1", MessageNumber: 2, MessageType: rocket.TypeSpeedDecreased},
		Event:    rocket.SpeedDecreased{By: 1000},
	})

	assert.Equal(t, rocket.MessageApplied, status)
	assert.Equal(t, rocket.Rocket{
		Channel:           "ch1",
		Status:            rocket.StatusLaunched,
		Type:              "Falcon-9",
		Mission:           "ARTEMIS",
		Speed:             2500,
		LastMessageNumber: 2,
	}, r)
}

func TestApplyMissionChanged(t *testing.T) {
	r := rocket.Rocket{
		Channel:           "ch1",
		Status:            rocket.StatusLaunched,
		Type:              "Falcon-9",
		Mission:           "ARTEMIS",
		Speed:             500,
		LastMessageNumber: 1,
	}

	status := r.Apply(rocket.Message{
		Metadata: rocket.Metadata{Channel: "ch1", MessageNumber: 2, MessageType: rocket.TypeMissionChanged},
		Event:    rocket.MissionChanged{NewMission: "SHUTTLE_MIR"},
	})

	assert.Equal(t, rocket.MessageApplied, status)
	assert.Equal(t, rocket.Rocket{
		Channel:           "ch1",
		Status:            rocket.StatusLaunched,
		Type:              "Falcon-9",
		Mission:           "SHUTTLE_MIR",
		Speed:             500,
		LastMessageNumber: 2,
	}, r)
}

func TestApplyExploded(t *testing.T) {
	r := rocket.Rocket{
		Channel:           "ch1",
		Status:            rocket.StatusLaunched,
		Type:              "Falcon-9",
		Mission:           "ARTEMIS",
		Speed:             500,
		LastMessageNumber: 1,
	}

	status := r.Apply(rocket.Message{
		Metadata: rocket.Metadata{Channel: "ch1", MessageNumber: 2, MessageType: rocket.TypeExploded},
		Event:    rocket.Exploded{Reason: "PRESSURE_VESSEL_FAILURE"},
	})

	assert.Equal(t, rocket.MessageApplied, status)
	assert.Equal(t, rocket.Rocket{
		Channel:           "ch1",
		Status:            rocket.StatusExploded,
		Type:              "Falcon-9",
		Mission:           "ARTEMIS",
		Speed:             500,
		ExplosionReason:   "PRESSURE_VESSEL_FAILURE",
		LastMessageNumber: 2,
	}, r)
}

func TestApplyEarlyMessageIsPending(t *testing.T) {
	r := rocket.NewRocket("ch1")

	status := r.Apply(rocket.Message{
		Metadata: rocket.Metadata{Channel: "ch1", MessageNumber: 2, MessageType: rocket.TypeSpeedIncreased},
		Event:    rocket.SpeedIncreased{By: 3000},
	})

	assert.Equal(t, rocket.MessagePending, status)
	assert.Equal(t, rocket.Rocket{
		Channel: "ch1",
		Status:  rocket.StatusAwaitingLaunch,
	}, r)
}

func TestApplyRepeatIsDuplicate(t *testing.T) {
	r := rocket.Rocket{
		Channel:           "ch1",
		Status:            rocket.StatusLaunched,
		Type:              "Falcon-9",
		Mission:           "ARTEMIS",
		Speed:             3500,
		LastMessageNumber: 2,
	}

	status := r.Apply(rocket.Message{
		Metadata: rocket.Metadata{Channel: "ch1", MessageNumber: 2, MessageType: rocket.TypeSpeedIncreased},
		Event:    rocket.SpeedIncreased{By: 3000},
	})
	assert.Equal(t, rocket.MessageDuplicate, status)

	status = r.Apply(rocket.Message{
		Metadata: rocket.Metadata{Channel: "ch1", MessageNumber: 1, MessageType: rocket.TypeLaunched},
		Event:    rocket.Launched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"},
	})
	assert.Equal(t, rocket.MessageDuplicate, status)

	assert.Equal(t, rocket.Rocket{
		Channel:           "ch1",
		Status:            rocket.StatusLaunched,
		Type:              "Falcon-9",
		Mission:           "ARTEMIS",
		Speed:             3500,
		LastMessageNumber: 2,
	}, r)
}
