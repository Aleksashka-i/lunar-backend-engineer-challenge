package rocket_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Aleksashka-i/lunar-backend-engineer-challenge/internal/rocket"
)

func TestMessageDecodesEventByType(t *testing.T) {
	tests := []struct {
		messageType, body string
		want              rocket.Event
	}{
		{"RocketLaunched", `{"type":"Falcon-9","launchSpeed":500,"mission":"ARTEMIS"}`, rocket.Launched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"}},
		{"RocketSpeedIncreased", `{"by":3000}`, rocket.SpeedIncreased{By: 3000}},
		{"RocketSpeedDecreased", `{"by":2500}`, rocket.SpeedDecreased{By: 2500}},
		{"RocketExploded", `{"reason":"PRESSURE_VESSEL_FAILURE"}`, rocket.Exploded{Reason: "PRESSURE_VESSEL_FAILURE"}},
		{"RocketMissionChanged", `{"newMission":"SHUTTLE_MIR"}`, rocket.MissionChanged{NewMission: "SHUTTLE_MIR"}},
	}
	for _, tt := range tests {
		t.Run(tt.messageType, func(t *testing.T) {
			data := `{"metadata":{"channel":"ch1","messageNumber":1,"messageType":"` + tt.messageType + `"},"message":` + tt.body + `}`
			var m rocket.Message
			require.NoError(t, json.Unmarshal([]byte(data), &m))
			assert.Equal(t, tt.want, m.Event)
		})
	}
}

func TestMessageRejectsUnknownType(t *testing.T) {
	data := `{"metadata":{"channel":"ch1","messageNumber":1,"messageType":"RocketLanded"},"message":{}}`
	var m rocket.Message
	assert.Error(t, json.Unmarshal([]byte(data), &m))
}
