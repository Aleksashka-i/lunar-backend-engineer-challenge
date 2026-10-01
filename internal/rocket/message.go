package rocket

import (
	"encoding/json"
	"fmt"
	"time"
)

// Message is a single event reported by a rocket.
type Message struct {
	Metadata Metadata `json:"metadata"`
	Event    Event    `json:"message"`
}

// Metadata identifies a message. Channel is the rocket ID and MessageNumber
// orders messages within a channel.
type Metadata struct {
	Channel       string    `json:"channel"`
	MessageNumber int       `json:"messageNumber"`
	MessageTime   time.Time `json:"messageTime"`
	MessageType   string    `json:"messageType"`
}

// Event is something that happened to a rocket and changes its state.
type Event interface {
	apply(s *State)
}

// Launched is sent once, when the rocket is launched.
type Launched struct {
	Type        string `json:"type"`
	LaunchSpeed int    `json:"launchSpeed"`
	Mission     string `json:"mission"`
}

// SpeedIncreased is sent when the rocket's speed increases.
type SpeedIncreased struct {
	By int `json:"by"`
}

// SpeedDecreased is sent when the rocket's speed decreases.
type SpeedDecreased struct {
	By int `json:"by"`
}

// Exploded is sent once, if the rocket explodes.
type Exploded struct {
	Reason string `json:"reason"`
}

// MissionChanged is sent when the rocket's mission changes.
type MissionChanged struct {
	NewMission string `json:"newMission"`
}

func (e Launched) apply(s *State) {
	s.Status = StatusLaunched
	s.Type = e.Type
	s.Speed = e.LaunchSpeed
	s.Mission = e.Mission
}

func (e SpeedIncreased) apply(s *State) { s.Speed += e.By }
func (e SpeedDecreased) apply(s *State) { s.Speed -= e.By }
func (e MissionChanged) apply(s *State) { s.Mission = e.NewMission }

func (e Exploded) apply(s *State) {
	s.Status = StatusExploded
	s.ExplosionReason = e.Reason
}

// decoders maps each messageType to a decoder for its event.
var decoders = map[string]func([]byte) (Event, error){
	"RocketLaunched":       decode[Launched],
	"RocketSpeedIncreased": decode[SpeedIncreased],
	"RocketSpeedDecreased": decode[SpeedDecreased],
	"RocketExploded":       decode[Exploded],
	"RocketMissionChanged": decode[MissionChanged],
}

func decode[E Event](data []byte) (Event, error) {
	var e E
	err := json.Unmarshal(data, &e)
	return e, err
}

// UnmarshalJSON decodes the event into the struct matching messageType.
func (m *Message) UnmarshalJSON(data []byte) error {
	var raw struct {
		Metadata Metadata        `json:"metadata"`
		Event    json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	decode, ok := decoders[raw.Metadata.MessageType]
	if !ok {
		return fmt.Errorf("unknown messageType %q", raw.Metadata.MessageType)
	}
	event, err := decode(raw.Event)
	if err != nil {
		return err
	}
	m.Metadata, m.Event = raw.Metadata, event
	return nil
}
