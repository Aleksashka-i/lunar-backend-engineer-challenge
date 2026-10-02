package rocket_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"lunar-backend-engineer-challenge/internal/rocket"
)

// mockStore is a testify mock of rocket.Store.
type mockStore struct{ mock.Mock }

func (s *mockStore) ProcessMessage(ctx context.Context, m rocket.Message, apply func(*rocket.Rocket, rocket.Message) rocket.MessageStatus) (rocket.MessageStatus, error) {
	args := s.Called(ctx, m, apply)
	return args.Get(0).(rocket.MessageStatus), args.Error(1)
}

func (s *mockStore) GetRocket(ctx context.Context, channel string) (rocket.Rocket, error) {
	args := s.Called(ctx, channel)
	return args.Get(0).(rocket.Rocket), args.Error(1)
}

func (s *mockStore) ListRockets(ctx context.Context) ([]rocket.Rocket, error) {
	args := s.Called(ctx)
	rockets, _ := args.Get(0).([]rocket.Rocket)
	return rockets, args.Error(1)
}

var launch = rocket.Message{
	Metadata: rocket.Metadata{Channel: "ch1", MessageNumber: 1, MessageType: rocket.TypeLaunched},
	Event:    rocket.Launched{Type: "Falcon-9", LaunchSpeed: 500, Mission: "ARTEMIS"},
}

func TestProcessMessage(t *testing.T) {
	store := new(mockStore)
	store.On("ProcessMessage", mock.Anything, launch, mock.Anything).Return(rocket.MessageApplied, nil)

	err := rocket.NewService(store).ProcessMessage(context.Background(), launch)

	require.NoError(t, err)
	store.AssertExpectations(t)
}

func TestProcessMessageRejectsInvalidMessage(t *testing.T) {
	store := new(mockStore)
	m := rocket.Message{Metadata: rocket.Metadata{Channel: "ch1", MessageNumber: 1}} // no event

	err := rocket.NewService(store).ProcessMessage(context.Background(), m)

	assert.ErrorIs(t, err, rocket.ErrInvalidMessage)
	store.AssertNotCalled(t, "ProcessMessage", mock.Anything, mock.Anything, mock.Anything)
}

func TestProcessMessageReturnsStoreError(t *testing.T) {
	failure := errors.New("database unavailable")
	store := new(mockStore)
	store.On("ProcessMessage", mock.Anything, launch, mock.Anything).Return(rocket.MessageStatus(""), failure)

	err := rocket.NewService(store).ProcessMessage(context.Background(), launch)

	assert.ErrorIs(t, err, failure)
}

func TestGet(t *testing.T) {
	want := rocket.Rocket{Channel: "ch1", Status: rocket.StatusLaunched, Speed: 500}
	store := new(mockStore)
	store.On("GetRocket", mock.Anything, "ch1").Return(want, nil)
	store.On("GetRocket", mock.Anything, "unknown").Return(rocket.Rocket{}, rocket.ErrNotFound)
	svc := rocket.NewService(store)

	got, err := svc.Get(context.Background(), "ch1")
	require.NoError(t, err)
	assert.Equal(t, want, got)

	_, err = svc.Get(context.Background(), "unknown")
	assert.ErrorIs(t, err, rocket.ErrNotFound)
}

func TestList(t *testing.T) {
	store := new(mockStore)
	store.On("ListRockets", mock.Anything).Return([]rocket.Rocket{
		{Channel: "a", Speed: 300},
		{Channel: "b", Speed: 100},
		{Channel: "c", Speed: 200},
	}, nil)

	rockets, err := rocket.NewService(store).List(context.Background(), rocket.SortBySpeed, true)
	require.NoError(t, err)

	var got []string
	for _, r := range rockets {
		got = append(got, r.Channel)
	}
	assert.Equal(t, []string{"a", "c", "b"}, got)
}

func TestListReturnsStoreError(t *testing.T) {
	failure := errors.New("database unavailable")
	store := new(mockStore)
	store.On("ListRockets", mock.Anything).Return(nil, failure)

	_, err := rocket.NewService(store).List(context.Background(), rocket.SortByChannel, false)

	assert.ErrorIs(t, err, failure)
}

func TestParseSortField(t *testing.T) {
	field, err := rocket.ParseSortField("speed")
	require.NoError(t, err)
	assert.Equal(t, rocket.SortBySpeed, field)

	field, err = rocket.ParseSortField("")
	require.NoError(t, err)
	assert.Equal(t, rocket.SortByChannel, field)

	_, err = rocket.ParseSortField("color")
	assert.Error(t, err)
}
