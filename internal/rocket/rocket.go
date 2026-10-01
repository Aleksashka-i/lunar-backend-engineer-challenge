// Package rocket holds the domain: rockets, the messages that change them, and
// the service that ingests and queries them.
package rocket

import "time"

// Status is where a rocket is in its lifecycle.
type Status string

const (
	StatusAwaitingLaunch Status = "awaiting_launch"
	StatusLaunched       Status = "launched"
	StatusExploded       Status = "exploded"
)

// MessageStatus is what happened to a received message.
type MessageStatus string

const (
	MessageApplied   MessageStatus = "applied"   // applied to the rocket's state
	MessagePending   MessageStatus = "pending"   // waiting for an earlier message
	MessageDuplicate MessageStatus = "duplicate" // already applied, ignored
)

// Rocket is the current state of a rocket, derived from its messages.
type Rocket struct {
	Channel           string    `json:"channel"`
	Status            Status    `json:"status"`
	Type              string    `json:"type"`
	Mission           string    `json:"mission"`
	Speed             int       `json:"speed"`
	ExplosionReason   string    `json:"explosionReason,omitempty"`
	LastMessageNumber int       `json:"lastMessageNumber"`
	LastMessageTime   time.Time `json:"lastMessageTime"`
}

// NewRocket creates a rocket for channel that has not launched yet.
func NewRocket(channel string) Rocket {
	return Rocket{Channel: channel, Status: StatusAwaitingLaunch}
}

// Apply applies m if it is the next message, and reports whether it was applied, is early or is a repeat.
func (r *Rocket) Apply(m Message) MessageStatus {
	switch number := m.Metadata.MessageNumber; {
	case number <= r.LastMessageNumber:
		return MessageDuplicate
	case number > r.LastMessageNumber+1:
		return MessagePending
	}
	m.Event.apply(r)
	r.LastMessageNumber = m.Metadata.MessageNumber
	r.LastMessageTime = m.Metadata.MessageTime
	return MessageApplied
}

// ApplyPending applies pending messages, sorted by number, that follow the last applied one, stopping at the first gap.
func (r *Rocket) ApplyPending(pending []Message) {
	for _, m := range pending {
		if r.Apply(m) == MessagePending {
			return
		}
	}
}
