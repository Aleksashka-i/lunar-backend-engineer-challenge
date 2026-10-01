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
	MessageDuplicate MessageStatus = "duplicate" // already applied or pending, ignored
)

// State is the current state of a rocket, derived from its messages.
type State struct {
	Channel           string    `json:"channel"`
	Status            Status    `json:"status"`
	Type              string    `json:"type"`
	Mission           string    `json:"mission"`
	Speed             int       `json:"speed"`
	ExplosionReason   string    `json:"explosionReason,omitempty"`
	LastMessageNumber int       `json:"lastMessageNumber"`
	LastMessageTime   time.Time `json:"lastMessageTime"`
}

// Rocket is a rocket's state plus the messages that arrived ahead of a missing
// predecessor and are waiting to be applied.
type Rocket struct {
	State
	Pending map[int]Message
}

// NewRocket creates a rocket for channel that has not launched yet.
func NewRocket(channel string) *Rocket {
	return &Rocket{State: State{Channel: channel, Status: StatusAwaitingLaunch}}
}

// Receive applies m in messageNumber order, holding early messages and ignoring duplicates.
func (r *Rocket) Receive(m Message) MessageStatus {
	number := m.Metadata.MessageNumber
	if _, pending := r.Pending[number]; pending || number <= r.LastMessageNumber {
		return MessageDuplicate
	}
	if r.Pending == nil {
		r.Pending = make(map[int]Message)
	}
	r.Pending[number] = m

	for {
		next, ok := r.Pending[r.LastMessageNumber+1]
		if !ok {
			break
		}
		delete(r.Pending, next.Metadata.MessageNumber)
		next.Event.apply(&r.State)
		r.LastMessageNumber = next.Metadata.MessageNumber
		r.LastMessageTime = next.Metadata.MessageTime
	}

	if number <= r.LastMessageNumber {
		return MessageApplied
	}
	return MessagePending
}
