package lights

import (
	"context"
	"encoding/json"
	"errors"

	"gv-api/internal/database/gvdb"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrDuplicateAddress is returned when a bulb with that BLE address is already registered.
var ErrDuplicateAddress = errors.New("that bulb is already added")

// errDuplicateID makes the service retry with a suffixed slug.
var errDuplicateID = errors.New("id already taken")

type Repository interface {
	List(ctx context.Context) ([]Light, error)
	Get(ctx context.Context, id string) (Light, error)
	Create(ctx context.Context, light Light) (Light, error)
	Update(ctx context.Context, light Light) (Light, error)
	Delete(ctx context.Context, id string) error
}

type PostgresRepository struct {
	q *gvdb.Queries
}

func NewRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{q: gvdb.New(pool)}
}

func (r *PostgresRepository) List(ctx context.Context) ([]Light, error) {
	rows, err := r.q.ListLights(ctx)
	if err != nil {
		return nil, err
	}
	lights := make([]Light, 0, len(rows))
	for _, row := range rows {
		lights = append(lights, Light{
			ID:                row.ID,
			Name:              row.Name,
			Model:             row.Model,
			Address:           row.Address,
			Protocol:          row.Protocol,
			SupportsColor:     row.SupportsColor,
			SupportsColorTemp: row.SupportsColorTemp,
			MinColorTemp:      row.MinColorTemp,
			MaxColorTemp:      row.MaxColorTemp,
			Options:           decodeOptions(row.Options),
		})
	}
	return lights, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id string) (Light, error) {
	row, err := r.q.GetLight(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Light{}, ErrNotFound
		}
		return Light{}, err
	}
	return Light{
		ID:                row.ID,
		Name:              row.Name,
		Model:             row.Model,
		Address:           row.Address,
		Protocol:          row.Protocol,
		SupportsColor:     row.SupportsColor,
		SupportsColorTemp: row.SupportsColorTemp,
		MinColorTemp:      row.MinColorTemp,
		MaxColorTemp:      row.MaxColorTemp,
		Options:           decodeOptions(row.Options),
	}, nil
}

func (r *PostgresRepository) Create(ctx context.Context, light Light) (Light, error) {
	row, err := r.q.CreateLight(ctx, gvdb.CreateLightParams{
		ID:                light.ID,
		Name:              light.Name,
		Model:             light.Model,
		Address:           light.Address,
		Protocol:          light.Protocol,
		SupportsColor:     light.SupportsColor,
		SupportsColorTemp: light.SupportsColorTemp,
		MinColorTemp:      light.MinColorTemp,
		MaxColorTemp:      light.MaxColorTemp,
		Options:           encodeOptions(light.Options),
	})
	if err != nil {
		return Light{}, classifyConflict(err)
	}
	light.ID = row.ID
	return light, nil
}

func (r *PostgresRepository) Update(ctx context.Context, light Light) (Light, error) {
	row, err := r.q.UpdateLight(ctx, gvdb.UpdateLightParams{
		ID:                light.ID,
		Name:              light.Name,
		Model:             light.Model,
		Protocol:          light.Protocol,
		SupportsColor:     light.SupportsColor,
		SupportsColorTemp: light.SupportsColorTemp,
		MinColorTemp:      light.MinColorTemp,
		MaxColorTemp:      light.MaxColorTemp,
		Options:           encodeOptions(light.Options),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Light{}, ErrNotFound
		}
		return Light{}, err
	}
	light.Address = row.Address
	return light, nil
}

func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	rows, err := r.q.DeleteLight(ctx, id)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// classifyConflict tells the two unique constraints apart; both arrive as SQLSTATE 23505.
func classifyConflict(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return err
	}
	switch pgErr.ConstraintName {
	case "lights_address_key":
		return ErrDuplicateAddress
	case "lights_pkey":
		return errDuplicateID
	default:
		return err
	}
}

func decodeOptions(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var options map[string]any
	if err := json.Unmarshal(raw, &options); err != nil {
		return nil
	}
	return options
}

func encodeOptions(options map[string]any) []byte {
	if len(options) == 0 {
		return []byte("{}")
	}
	raw, err := json.Marshal(options)
	if err != nil {
		return []byte("{}")
	}
	return raw
}
