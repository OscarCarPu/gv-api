package varieties_test

import (
	"context"
	"testing"

	"gv-api/internal/actor"
	"gv-api/internal/testutil"
	"gv-api/internal/varieties"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// actorCtx carries actor info so the audit trigger can stamp it onto
// weed_varieties_history rows (the repository pins it via SET LOCAL).
func actorCtx() context.Context {
	return actor.WithInfo(context.Background(), actor.Info{
		IP:        "203.0.113.7",
		UserAgent: "repo-test",
		DeviceID:  "test-device",
		TokenKind: "full",
	})
}

func newRepo(t *testing.T) (*varieties.PostgresRepository, *pgxpool.Pool) {
	t.Helper()
	pool := testutil.NewPool(t)
	testutil.Truncate(t, pool, "weed_varieties_history", "weed_varieties")
	return varieties.NewRepository(pool), pool
}

func strPtr(s string) *string { return &s }

func TestIntegration_CreateGet_RoundTrip(t *testing.T) {
	ctx := actorCtx()
	repo, _ := newRepo(t)

	created, err := repo.Create(ctx, varieties.CreateVarietyRequest{
		Name:     "Amnesia",
		Scent:    7,
		Flavor:   8,
		Power:    6.5,
		Quality:  7.5,
		Price:    10,
		Comments: strPtr("nice"),
		Judge:    "Oscar",
	})
	require.NoError(t, err)
	require.NotZero(t, created.ID)

	got, err := repo.Get(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, "Amnesia", got.Name)
	require.Equal(t, float32(7), got.Scent)
	require.Equal(t, float32(8), got.Flavor)
	require.Equal(t, float32(6.5), got.Power)
	require.Equal(t, float32(7.5), got.Quality)
	require.Equal(t, float32(10), got.Price)
	require.Equal(t, "Oscar", got.Judge)
	require.NotNil(t, got.Comments)
	require.Equal(t, "nice", *got.Comments)
}

func TestIntegration_NotFound(t *testing.T) {
	ctx := actorCtx()
	repo, _ := newRepo(t)

	_, err := repo.Get(ctx, 99999)
	require.ErrorIs(t, err, varieties.ErrNotFound)

	_, err = repo.Update(ctx, varieties.UpdateVarietyRequest{ID: 99999, Name: "X", Judge: "J"})
	require.ErrorIs(t, err, varieties.ErrNotFound)

	// Delete is a soft-delete exec with no rows-affected check; a missing id
	// is not an error. This documents the current contract.
	require.NoError(t, repo.Delete(ctx, 99999))
}

func TestIntegration_Update_WritesHistoryWithActor(t *testing.T) {
	ctx := actorCtx()
	repo, pool := newRepo(t)

	created, err := repo.Create(ctx, varieties.CreateVarietyRequest{Name: "Original", Judge: "Oscar"})
	require.NoError(t, err)

	updated, err := repo.Update(ctx, varieties.UpdateVarietyRequest{
		ID:    created.ID,
		Name:  "Renamed",
		Scent: 5,
		Judge: "Pep",
	})
	require.NoError(t, err)
	require.Equal(t, "Renamed", updated.Name)
	require.Equal(t, "Pep", updated.Judge)

	var op, ip string
	err = pool.QueryRow(context.Background(),
		`SELECT op, actor_ip FROM weed_varieties_history
		 WHERE variety_id = $1 AND op = 'UPDATE'
		 ORDER BY history_id DESC LIMIT 1`, created.ID).Scan(&op, &ip)
	require.NoError(t, err)
	require.Equal(t, "UPDATE", op)
	require.Equal(t, "203.0.113.7", ip, "audit trigger must see the actor pinned by withActorTx")
}

func TestIntegration_List(t *testing.T) {
	ctx := actorCtx()
	repo, _ := newRepo(t)

	for _, name := range []string{"A", "B", "C"} {
		_, err := repo.Create(ctx, varieties.CreateVarietyRequest{Name: name, Judge: "Oscar"})
		require.NoError(t, err)
	}

	got, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, got, 3)
}

func TestIntegration_Delete_HidesFromGet(t *testing.T) {
	ctx := actorCtx()
	repo, _ := newRepo(t)

	created, err := repo.Create(ctx, varieties.CreateVarietyRequest{Name: "Gone", Judge: "Oscar"})
	require.NoError(t, err)

	require.NoError(t, repo.Delete(ctx, created.ID))

	_, err = repo.Get(ctx, created.ID)
	require.ErrorIs(t, err, varieties.ErrNotFound, "soft-deleted varieties must not be returned")
}
