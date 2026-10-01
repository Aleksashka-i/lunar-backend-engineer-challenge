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

// ErrInvalidMessage is returned when a message cannot be processed.
var ErrInvalidMessage = errors.New("invalid message")

// Store persists rockets and the messages waiting to be applied to them.
type Store interface {
	// WithTx runs fn in one transaction, committing if fn returns nil and rolling back otherwise.
	WithTx(ctx context.Context, fn func(tx TxStore) error) error
	// GetRocket returns one rocket, or ErrNotFound.
	GetRocket(ctx context.Context, channel string) (Rocket, error)
	// ListRockets returns all rockets in no particular order.
	ListRockets(ctx context.Context) ([]Rocket, error)
}

// TxStore is the Store inside a transaction; it is only valid during the WithTx call that provides it.
type TxStore interface {
	// GetRocket returns one rocket, or ErrNotFound.
	GetRocket(ctx context.Context, channel string) (Rocket, error)
	// GetPendingMessages returns the messages waiting for channel's rocket, ordered by number.
	GetPendingMessages(ctx context.Context, channel string) ([]Message, error)
	// SaveRocket creates or replaces a rocket.
	SaveRocket(ctx context.Context, r Rocket) error
	// SavePendingMessage stores m to wait for an earlier message; a message already pending is kept.
	SavePendingMessage(ctx context.Context, m Message) error
	// DeletePendingMessages deletes channel's pending messages numbered up to throughNumber.
	DeletePendingMessages(ctx context.Context, channel string, throughNumber int) error
}

// SortField is the rocket attribute a list is sorted by.
type SortField string

const (
	SortByChannel           SortField = "channel"
	SortByType              SortField = "type"
	SortByMission           SortField = "mission"
	SortBySpeed             SortField = "speed"
	SortByStatus            SortField = "status"
	SortByLastMessageNumber SortField = "lastMessageNumber"
)

// ParseSortField parses a sort field, defaulting to SortByChannel when empty.
func ParseSortField(s string) (SortField, error) {
	switch f := SortField(s); f {
	case "":
		return SortByChannel, nil
	case SortByChannel, SortByType, SortByMission, SortBySpeed, SortByStatus, SortByLastMessageNumber:
		return f, nil
	default:
		return "", fmt.Errorf("unknown sort field %q", s)
	}
}

// Service processes messages and queries rockets.
type Service struct {
	store Store
}

// NewService creates a Service backed by store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// ProcessMessage applies m and the pending messages after it, or stores m as pending if it is early, in one transaction.
func (s *Service) ProcessMessage(ctx context.Context, m Message) error {
	if err := validate(m); err != nil {
		return err
	}
	channel := m.Metadata.Channel

	return s.store.WithTx(ctx, func(tx TxStore) error {
		r, err := tx.GetRocket(ctx, channel)
		if errors.Is(err, ErrNotFound) {
			r = NewRocket(channel)
		} else if err != nil {
			return err
		}

		switch r.Apply(m) {
		case MessageDuplicate:
			return nil
		case MessagePending:
			if err := tx.SavePendingMessage(ctx, m); err != nil {
				return err
			}
		case MessageApplied:
			pending, err := tx.GetPendingMessages(ctx, channel)
			if err != nil {
				return err
			}
			r.ApplyPending(pending)
			if err := tx.DeletePendingMessages(ctx, channel, r.LastMessageNumber); err != nil {
				return err
			}
		}
		return tx.SaveRocket(ctx, r)
	})
}

// Get returns one rocket.
func (s *Service) Get(ctx context.Context, channel string) (Rocket, error) {
	return s.store.GetRocket(ctx, channel)
}

// List returns all rockets sorted by field, descending if desc.
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

// statusOrder sorts statuses by lifecycle rather than alphabetically.
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
