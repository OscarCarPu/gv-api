package rutas_test

import (
	"context"
	"errors"
	"testing"

	"gv-api/internal/rutas"
	"gv-api/internal/rutas/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// The service is a pure pass-through; these tests pin that arguments are
// forwarded unchanged and errors propagate.

func TestService_List(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	want := []rutas.ConcelloMark{{ID: 1, Name: "Arzúa"}}
	repo.EXPECT().List(mock.Anything).Return(want, nil)

	got, err := rutas.NewService(repo).List(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestService_Get(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Get(mock.Anything, int32(7)).Return(rutas.ConcelloMark{}, rutas.ErrNotFound)

	_, err := rutas.NewService(repo).Get(context.Background(), 7)
	assert.ErrorIs(t, err, rutas.ErrNotFound)
}

func TestService_Create(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	req := rutas.CreateMarkRequest{Name: "Melide", VisitedOn: "2026-06-11"}
	repo.EXPECT().Create(mock.Anything, req).Return(rutas.ConcelloMark{ID: 2, Name: "Melide"}, nil)

	got, err := rutas.NewService(repo).Create(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, int32(2), got.ID)
}

func TestService_Update(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	req := rutas.UpdateMarkRequest{ID: 3, VisitedOn: "2026-06-11"}
	repo.EXPECT().Update(mock.Anything, req).Return(rutas.ConcelloMark{}, errors.New("db error"))

	_, err := rutas.NewService(repo).Update(context.Background(), req)
	assert.EqualError(t, err, "db error")
}

func TestService_Delete(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Delete(mock.Anything, int32(4)).Return(nil)

	assert.NoError(t, rutas.NewService(repo).Delete(context.Background(), 4))
}
