package rocket_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"lunar-backend-engineer-challenge/internal/rocket"
)

func TestMessageDecodesEventByType(t *testing.T) {
	tests := []struct {
		messageType rocket.MessageType
		body        string
		want        rocket.Event
	}{
		{rocket.TypeLaunched, `{"type":"Falcon-9","launchSpeed":500,"mission":"ARTEMIS"}`, rocket.Launched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"}},
		{rocket.TypeSpeedIncreased, `{"by":3000}`, rocket.SpeedIncreased{By: 3000}},
		{rocket.TypeSpeedDecreased, `{"by":2500}`, rocket.SpeedDecreased{By: 2500}},
		{rocket.TypeExploded, `{"reason":"PRESSURE_VESSEL_FAILURE"}`, rocket.Exploded{Reason: "PRESSURE_VESSEL_FAILURE"}},
		{rocket.TypeMissionChanged, `{"newMission":"SHUTTLE_MIR"}`, rocket.MissionChanged{NewMission: "SHUTTLE_MIR"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.messageType), func(t *testing.T) {
			data := `{"metadata":{"channel":"ch1","messageNumber":1,"messageType":"` + string(tt.messageType) + `"},"message":` + tt.body + `}`
			var m rocket.Message
			require.NoError(t, json.Unmarshal([]byte(data), &m))
			assert.Equal(t, tt.want, m.Event)
		})
	}
}

func TestValidate(t *testing.T) {
	valid := rocket.Metadata{Channel: "ch1", MessageNumber: 2, MessageType: rocket.TypeSpeedIncreased}
	noChannel, zeroNumber := valid, valid
	noChannel.Channel = ""
	zeroNumber.MessageNumber = 0

	assert.NoError(t, rocket.Message{Metadata: valid, Event: rocket.SpeedIncreased{By: 1}}.Validate())

	tests := map[string]rocket.Message{
		"missing channel":     {Metadata: noChannel, Event: rocket.SpeedIncreased{By: 1}},
		"zero message number": {Metadata: zeroNumber, Event: rocket.SpeedIncreased{By: 1}},
		"missing event":       {Metadata: valid},
	}
	for name, m := range tests {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, m.Validate(), rocket.ErrInvalidMessage)
		})
	}
}

func TestDecodingRejectsUnknownOrMalformedEvents(t *testing.T) {
	tests := map[string]string{
		"unknown message type": `{"metadata":{"channel":"ch1","messageNumber":2,"messageType":"RocketLanded"},"message":{}}`,
		"malformed event":      `{"metadata":{"channel":"ch1","messageNumber":2,"messageType":"RocketSpeedIncreased"},"message":{"by":"fast"}}`,
		"malformed metadata":   `{"metadata":"ch1","message":{}}`,
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			var m rocket.Message
			assert.ErrorIs(t, json.Unmarshal([]byte(data), &m), rocket.ErrInvalidMessage)
		})
	}
}

func TestMessageRoundTripsThroughJSON(t *testing.T) {
	m := rocket.Message{
		Metadata: rocket.Metadata{Channel: "ch1", MessageNumber: 2, MessageType: rocket.TypeSpeedIncreased},
		Event:    rocket.SpeedIncreased{By: 3000},
	}
	data, err := json.Marshal(m)
	require.NoError(t, err)

	var got rocket.Message
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, m, got)
}
