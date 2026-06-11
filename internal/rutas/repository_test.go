package rutas_test

import (
	"context"
	"testing"
	"time"

	"gv-api/internal/rutas"
	"gv-api/internal/testutil"

	"github.com/stretchr/testify/require"
)

func newRepo(t *testing.T) *rutas.PostgresRepository {
	t.Helper()
	pool := testutil.NewPool(t)
	testutil.Truncate(t, pool, "concello_marks")
	return rutas.NewRepository(pool)
}

func TestIntegration_CreateGet_RoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	created, err := repo.Create(ctx, rutas.CreateMarkRequest{
		Name:        "Arzúa",
		VisitedOn:   "2026-06-11",
		Description: "queixo",
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)

	got, err := repo.Get(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "Arzúa", got.Name)
	require.Equal(t, "queixo", got.Description)
	require.Equal(t, "2026-06-11", got.VisitedOn.Format("2006-01-02"))
}

func TestIntegration_Create_InvalidDate(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	_, err := repo.Create(ctx, rutas.CreateMarkRequest{Name: "X", VisitedOn: "11/06/2026"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "must be YYYY-MM-DD")
}

func TestIntegration_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	_, err := repo.Get(ctx, 99999)
	require.ErrorIs(t, err, rutas.ErrNotFound)
}

func TestIntegration_List(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	for _, name := range []string{"Arzúa", "Melide", "Sobrado"} {
		_, err := repo.Create(ctx, rutas.CreateMarkRequest{Name: name, VisitedOn: "2026-06-11"})
		require.NoError(t, err)
	}

	marks, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, marks, 3)
}

func TestIntegration_Update_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	_, err := repo.Update(ctx, rutas.UpdateMarkRequest{ID: 99999, VisitedOn: "2026-06-11"})
	require.ErrorIs(t, err, rutas.ErrNotFound)
}

func TestIntegration_Update_KeepsName(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	created, err := repo.Create(ctx, rutas.CreateMarkRequest{
		Name:        "Melide",
		VisitedOn:   "2026-06-01",
		Description: "old",
	})
	require.NoError(t, err)

	updated, err := repo.Update(ctx, rutas.UpdateMarkRequest{
		ID:          created.ID,
		VisitedOn:   "2026-06-11",
		Description: "new",
	})
	require.NoError(t, err)
	require.Equal(t, "Melide", updated.Name, "UpdateMarkRequest has no Name; it must stay intact")
	require.Equal(t, "new", updated.Description)
	require.Equal(t, "2026-06-11", updated.VisitedOn.Format("2006-01-02"))
}

func TestIntegration_Delete(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	created, err := repo.Create(ctx, rutas.CreateMarkRequest{Name: "Toques", VisitedOn: "2026-06-11"})
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, created.ID))

	_, err = repo.Get(ctx, created.ID)
	require.ErrorIs(t, err, rutas.ErrNotFound)

	// Delete of a missing id does not report an error — the DELETE is a plain
	// exec with no rows-affected check. This documents the current contract.
	require.NoError(t, repo.Delete(ctx, created.ID))
}

func TestIntegration_VisitedOn_IsDateOnly(t *testing.T) {
	ctx := context.Background()
	repo := newRepo(t)

	created, err := repo.Create(ctx, rutas.CreateMarkRequest{Name: "Arzúa", VisitedOn: "2026-06-11"})
	require.NoError(t, err)
	require.Equal(t, time.Date(2026, 6, 11, 0, 0, 0, 0, created.VisitedOn.Location()), created.VisitedOn)
}
