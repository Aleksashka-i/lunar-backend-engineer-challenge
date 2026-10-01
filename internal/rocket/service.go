package rocket

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
)

// ErrNotFound is returned when a rocket does not exist.
var ErrNotFound = errors.New("rocket not found")

// ErrInvalidMessage is returned when a message cannot be ingested.
var ErrInvalidMessage = errors.New("invalid message")

// Repository stores rockets.
type Repository interface {
	// Update atomically applies fn to the rocket for channel, creating it if needed.
	Update(ctx context.Context, channel string, fn func(*Rocket)) error
	// Get returns the state of one rocket, or ErrNotFound.
	Get(ctx context.Context, channel string) (State, error)
	// List returns the states of all rockets in no particular order.
	List(ctx context.Context) ([]State, error)
}

// SortField is the rocket attribute a list is sorted by.
type SortField string

const (
	SortByChannel SortField = "channel"
	SortByType    SortField = "type"
	SortByMission SortField = "mission"
	SortBySpeed   SortField = "speed"
)

// ParseSortField parses a sort field, defaulting to SortByChannel when empty.
func ParseSortField(s string) (SortField, error) {
	switch f := SortField(s); f {
	case "":
		return SortByChannel, nil
	case SortByChannel, SortByType, SortByMission, SortBySpeed:
		return f, nil
	default:
		return "", fmt.Errorf("unknown sort field %q", s)
	}
}

// Service ingests messages and queries rocket states.
type Service struct {
	repo Repository
}

// NewService creates a Service backed by repo.
func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Ingest validates a message, passes it to its rocket and reports its status.
func (s *Service) Ingest(ctx context.Context, m Message) (MessageStatus, error) {
	if err := validate(m); err != nil {
		return "", err
	}
	var status MessageStatus
	err := s.repo.Update(ctx, m.Metadata.Channel, func(r *Rocket) { status = r.Receive(m) })
	if err != nil {
		return "", err
	}
	return status, nil
}

// Get returns the state of one rocket.
func (s *Service) Get(ctx context.Context, channel string) (State, error) {
	return s.repo.Get(ctx, channel)
}

// List returns the states of all rockets sorted by field, descending if desc.
func (s *Service) List(ctx context.Context, field SortField, desc bool) ([]State, error) {
	states, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(states, func(a, b State) int {
		c := compare(a, b, field)
		if c == 0 {
			c = cmp.Compare(a.Channel, b.Channel)
		}
		if desc {
			return -c
		}
		return c
	})
	return states, nil
}

func compare(a, b State, field SortField) int {
	switch field {
	case SortByType:
		return cmp.Compare(a.Type, b.Type)
	case SortByMission:
		return cmp.Compare(a.Mission, b.Mission)
	case SortBySpeed:
		return cmp.Compare(a.Speed, b.Speed)
	default:
		return cmp.Compare(a.Channel, b.Channel)
	}
}

func validate(m Message) error {
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
