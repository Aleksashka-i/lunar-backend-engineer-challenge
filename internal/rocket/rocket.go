// Package rocket contains the domain model (rockets and the messages that change them) and the service that processes and queries them.
package rocket

import "time"

// Status is a rocket's lifecycle stage.
type Status string

const (
	StatusAwaitingLaunch Status = "awaiting_launch"
	StatusLaunched       Status = "launched"
	StatusExploded       Status = "exploded"
)

// MessageStatus is the outcome of applying a message to a rocket.
type MessageStatus string

const (
	MessageApplied   MessageStatus = "applied"   // applied to the rocket's state
	MessagePending   MessageStatus = "pending"   // awaiting an earlier message
	MessageDuplicate MessageStatus = "duplicate" // already applied; ignored
)

// Rocket is the current state of a rocket, derived from its messages.
type Rocket struct {
	Channel           string    `json:"channel"`
	Status            Status    `json:"status"`
	Type              string    `json:"type"`
	Mission           string    `json:"mission"`
	Speed             int       `json:"speed"`
	ExplosionReason   string    `json:"explosionReason,omitempty"`
	LastMessageNumber int64     `json:"lastMessageNumber"`
	LastMessageTime   time.Time `json:"lastMessageTime"`
	// PendingMessages counts messages that arrived early and wait for a gap to fill; set by the store on read.
	PendingMessages int `json:"pendingMessages"`
}

// NewRocket returns an unlaunched rocket for channel.
func NewRocket(channel string) Rocket {
	return Rocket{Channel: channel, Status: StatusAwaitingLaunch}
}

// Apply applies m if it is the next message in sequence and reports the outcome.
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
