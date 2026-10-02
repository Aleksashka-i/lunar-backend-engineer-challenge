package rocket

import (
	"encoding/json"
	"fmt"
	"time"
)

// MessageType identifies the kind of event a message carries.
type MessageType string

const (
	TypeLaunched       MessageType = "RocketLaunched"
	TypeSpeedIncreased MessageType = "RocketSpeedIncreased"
	TypeSpeedDecreased MessageType = "RocketSpeedDecreased"
	TypeExploded       MessageType = "RocketExploded"
	TypeMissionChanged MessageType = "RocketMissionChanged"
)

// Message is a single event reported by a rocket.
type Message struct {
	Metadata Metadata `json:"metadata"`
	Event    Event    `json:"message"`
}

// Metadata identifies a message: Channel is the rocket ID, and MessageNumber orders
// messages within that channel.
type Metadata struct {
	Channel       string      `json:"channel"`
	MessageNumber int64       `json:"messageNumber"`
	MessageTime   time.Time   `json:"messageTime"`
	MessageType   MessageType `json:"messageType"`
}

// Validate checks that m identifies a rocket and message and carries an event.
func (m Message) Validate() error {
	switch {
	case m.Metadata.Channel == "":
		return fmt.Errorf("%w: missing channel", ErrInvalidMessage)
	case m.Metadata.MessageNumber < 1:
		return fmt.Errorf("%w: messageNumber must be positive", ErrInvalidMessage)
	case m.Event == nil:
		return fmt.Errorf("%w: missing event", ErrInvalidMessage)
	}
	return nil
}

// UnmarshalJSON decodes the metadata and the event selected by messageType.
func (m *Message) UnmarshalJSON(data []byte) error {
	var raw struct {
		Metadata Metadata        `json:"metadata"`
		Event    json.RawMessage `json:"message"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}
	event, err := decodeEvent(raw.Metadata.MessageType, raw.Event)
	if err != nil {
		return err
	}
	m.Metadata, m.Event = raw.Metadata, event
	return nil
}

// Event is a change to a rocket's state.
type Event interface {
	apply(r *Rocket)
}

// Launched reports that the rocket was launched; it is sent once.
type Launched struct {
	Type        string `json:"type"`
	LaunchSpeed int    `json:"launchSpeed"`
	Mission     string `json:"mission"`
}

func (e Launched) apply(r *Rocket) {
	r.Status = StatusLaunched
	r.Type = e.Type
	r.Speed = e.LaunchSpeed
	r.Mission = e.Mission
}

// SpeedIncreased reports an increase in the rocket's speed.
type SpeedIncreased struct {
	By int `json:"by"`
}

func (e SpeedIncreased) apply(r *Rocket) { r.Speed += e.By }

// SpeedDecreased reports a decrease in the rocket's speed.
type SpeedDecreased struct {
	By int `json:"by"`
}

func (e SpeedDecreased) apply(r *Rocket) { r.Speed -= e.By }

// Exploded reports that the rocket exploded; it is sent at most once.
type Exploded struct {
	Reason string `json:"reason"`
}

func (e Exploded) apply(r *Rocket) {
	r.Status = StatusExploded
	r.ExplosionReason = e.Reason
}

// MissionChanged reports a change of the rocket's mission.
type MissionChanged struct {
	NewMission string `json:"newMission"`
}

func (e MissionChanged) apply(r *Rocket) { r.Mission = e.NewMission }

// decoders maps each message type to the decoder for its event.
var decoders = map[MessageType]func([]byte) (Event, error){
	TypeLaunched:       decode[Launched],
	TypeSpeedIncreased: decode[SpeedIncreased],
	TypeSpeedDecreased: decode[SpeedDecreased],
	TypeExploded:       decode[Exploded],
	TypeMissionChanged: decode[MissionChanged],
}

// decodeEvent decodes data into the event of the given type.
func decodeEvent(t MessageType, data []byte) (Event, error) {
	decode, ok := decoders[t]
	if !ok {
		return nil, fmt.Errorf("%w: unknown messageType %q", ErrInvalidMessage, t)
	}
	event, err := decode(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidMessage, err)
	}
	return event, nil
}

func decode[E Event](data []byte) (Event, error) {
	var e E
	err := json.Unmarshal(data, &e)
	return e, err
}
