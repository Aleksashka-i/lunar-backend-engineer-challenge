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

// ErrInvalidMessage is returned when a message fails validation.
var ErrInvalidMessage = errors.New("invalid message")

// Store persists rockets and the messages awaiting earlier ones.
type Store interface {
	// ProcessMessage atomically applies m and any pending messages that follow it, using apply; calls for the same rocket must be serialized.
	ProcessMessage(ctx context.Context, m Message, apply func(r *Rocket, m Message) MessageStatus) (MessageStatus, error)
	// GetRocket returns one rocket, or ErrNotFound.
	GetRocket(ctx context.Context, channel string) (Rocket, error)
	// ListRockets returns all rockets in unspecified order.
	ListRockets(ctx context.Context) ([]Rocket, error)
}

// SortField is a rocket attribute to sort by.
type SortField string

const (
	SortByChannel           SortField = "channel"
	SortByType              SortField = "type"
	SortByMission           SortField = "mission"
	SortBySpeed             SortField = "speed"
	SortByStatus            SortField = "status"
	SortByLastMessageNumber SortField = "lastMessageNumber"
	SortByPendingMessages   SortField = "pendingMessages"
)

// ParseSortField parses s as a SortField; an empty string yields SortByChannel.
func ParseSortField(s string) (SortField, error) {
	switch f := SortField(s); f {
	case "":
		return SortByChannel, nil
	case SortByChannel, SortByType, SortByMission, SortBySpeed, SortByStatus, SortByLastMessageNumber, SortByPendingMessages:
		return f, nil
	default:
		return "", fmt.Errorf("unknown sort field %q", s)
	}
}

// Service processes incoming messages and serves rocket queries.
type Service struct {
	store Store
}

// NewService returns a Service backed by store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// ProcessMessage validates m and applies it to its rocket.
func (s *Service) ProcessMessage(ctx context.Context, m Message) error {
	if err := m.Validate(); err != nil {
		return err
	}
	_, err := s.store.ProcessMessage(ctx, m, (*Rocket).Apply)
	return err
}

// Get returns the rocket for channel, or ErrNotFound.
func (s *Service) Get(ctx context.Context, channel string) (Rocket, error) {
	return s.store.GetRocket(ctx, channel)
}

// List returns all rockets sorted by field, in descending order if desc is true.
func (s *Service) List(ctx context.Context, field SortField, desc bool) ([]Rocket, error) {
	rockets, err := s.store.ListRockets(ctx)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(rockets, func(a, b Rocket) int {
		c := compare(a, b, field)
		if c == 0 {
			c = cmp.Compare(a.Channel, b.Channel)
		}
		if desc {
			return -c
		}
		return c
	})
	return rockets, nil
}

// statusOrder orders statuses by lifecycle stage rather than alphabetically.
var statusOrder = map[Status]int{StatusAwaitingLaunch: 0, StatusLaunched: 1, StatusExploded: 2}

func compare(a, b Rocket, field SortField) int {
	switch field {
	case SortByType:
		return cmp.Compare(a.Type, b.Type)
	case SortByMission:
		return cmp.Compare(a.Mission, b.Mission)
	case SortBySpeed:
		return cmp.Compare(a.Speed, b.Speed)
	case SortByStatus:
		return cmp.Compare(statusOrder[a.Status], statusOrder[b.Status])
	case SortByLastMessageNumber:
		return cmp.Compare(a.LastMessageNumber, b.LastMessageNumber)
	case SortByPendingMessages:
		return cmp.Compare(a.PendingMessages, b.PendingMessages)
	default:
		return cmp.Compare(a.Channel, b.Channel)
	}
}
