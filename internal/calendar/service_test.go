package calendar_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"gv-api/internal/calendar"
	"gv-api/internal/calendar/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type stubPlanSync struct {
	synced   []string
	detached []string
}

func (s *stubPlanSync) SyncEventTime(_ context.Context, ref string, _, _ time.Time) error {
	s.synced = append(s.synced, ref)
	return nil
}

func (s *stubPlanSync) DetachEvent(_ context.Context, ref string) error {
	s.detached = append(s.detached, ref)
	return nil
}

var t0 = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)

func TestCreateEvent_Validation(t *testing.T) {
	svc := calendar.NewService(mocks.NewMockRepository(t), &stubPlanSync{})

	_, err := svc.CreateEvent(context.Background(), calendar.CreateEventRequest{StartTime: t0, EndTime: t0.Add(time.Hour)})
	assert.ErrorIs(t, err, calendar.ErrInvalid)

	_, err = svc.CreateEvent(context.Background(), calendar.CreateEventRequest{Name: "x", StartTime: t0, EndTime: t0})
	assert.ErrorIs(t, err, calendar.ErrInvalid)
}

func TestListEvents_RejectsBadRanges(t *testing.T) {
	svc := calendar.NewService(mocks.NewMockRepository(t), &stubPlanSync{})

	_, err := svc.ListEvents(context.Background(), t0, t0)
	assert.ErrorIs(t, err, calendar.ErrInvalid)

	_, err = svc.ListEvents(context.Background(), t0, t0.AddDate(3, 0, 0))
	assert.ErrorIs(t, err, calendar.ErrInvalid)
}

func TestUpdateEvent_MovesTheLinkedPlanBlock(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	plan := &stubPlanSync{}
	svc := calendar.NewService(repo, plan)

	current := calendar.Event{ID: 7, Name: "x", StartTime: t0, EndTime: t0.Add(time.Hour)}
	newEnd := t0.Add(2 * time.Hour)
	req := calendar.UpdateEventRequest{EndTime: &newEnd}
	repo.EXPECT().Get(mock.Anything, int32(7)).Return(current, nil)
	repo.EXPECT().Update(mock.Anything, int32(7), req).Return(calendar.Event{ID: 7, Name: "x", StartTime: t0, EndTime: newEnd}, nil)

	_, err := svc.UpdateEvent(context.Background(), 7, req)
	require.NoError(t, err)
	assert.Equal(t, []string{"7"}, plan.synced)
}

func TestUpdateEvent_NameOnlyLeavesThePlanAlone(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	plan := &stubPlanSync{}
	svc := calendar.NewService(repo, plan)

	name := "renamed"
	req := calendar.UpdateEventRequest{Name: &name}
	repo.EXPECT().Get(mock.Anything, int32(7)).Return(calendar.Event{ID: 7, StartTime: t0, EndTime: t0.Add(time.Hour)}, nil)
	repo.EXPECT().Update(mock.Anything, int32(7), req).Return(calendar.Event{ID: 7, Name: name}, nil)

	_, err := svc.UpdateEvent(context.Background(), 7, req)
	require.NoError(t, err)
	assert.Empty(t, plan.synced)
}

func TestUpdateEvent_RejectsAnEndBeforeTheStart(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	svc := calendar.NewService(repo, &stubPlanSync{})

	earlier := t0.Add(-time.Hour)
	repo.EXPECT().Get(mock.Anything, int32(7)).Return(calendar.Event{ID: 7, StartTime: t0, EndTime: t0.Add(time.Hour)}, nil)

	_, err := svc.UpdateEvent(context.Background(), 7, calendar.UpdateEventRequest{EndTime: &earlier})
	assert.ErrorIs(t, err, calendar.ErrInvalid)
}

func TestDeleteEvent_DetachesThePlanBlock(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	plan := &stubPlanSync{}
	svc := calendar.NewService(repo, plan)

	repo.EXPECT().Delete(mock.Anything, int32(7)).Return(nil)

	require.NoError(t, svc.DeleteEvent(context.Background(), 7))
	assert.Equal(t, []string{strconv.Itoa(7)}, plan.detached)
}
