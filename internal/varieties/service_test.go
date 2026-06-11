package varieties_test

import (
	"context"
	"errors"
	"testing"

	"gv-api/internal/varieties"
	"gv-api/internal/varieties/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// The service is a pure pass-through; these tests pin that arguments are
// forwarded unchanged and errors propagate.

func TestService_Get(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Get(mock.Anything, int32(7)).Return(varieties.Variety{ID: 7}, nil)

	got, err := varieties.NewService(repo).Get(context.Background(), 7)
	assert.NoError(t, err)
	assert.Equal(t, int32(7), got.ID)
}

func TestService_List(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().List(mock.Anything).Return(nil, errors.New("db error"))

	_, err := varieties.NewService(repo).List(context.Background())
	assert.EqualError(t, err, "db error")
}

func TestService_Create(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	req := varieties.CreateVarietyRequest{Name: "Amnesia", Judge: "Oscar"}
	repo.EXPECT().Create(mock.Anything, req).Return(varieties.Variety{ID: 1}, nil)

	got, err := varieties.NewService(repo).Create(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, int32(1), got.ID)
}

func TestService_Update(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	req := varieties.UpdateVarietyRequest{ID: 2, Name: "Renamed", Judge: "Oscar"}
	repo.EXPECT().Update(mock.Anything, req).Return(varieties.Variety{}, varieties.ErrNotFound)

	_, err := varieties.NewService(repo).Update(context.Background(), req)
	assert.ErrorIs(t, err, varieties.ErrNotFound)
}

func TestService_Delete(t *testing.T) {
	repo := mocks.NewMockRepository(t)
	repo.EXPECT().Delete(mock.Anything, int32(3)).Return(nil)

	assert.NoError(t, varieties.NewService(repo).Delete(context.Background(), 3))
}
