package assistant

import (
	"context"
	"time"

	"gv-api/internal/assistant/llm"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
)

// UsageRepository persists and aggregates LLM usage rows.
type UsageRepository struct {
	pool *pgxpool.Pool
}

func NewUsageRepository(pool *pgxpool.Pool) *UsageRepository {
	return &UsageRepository{pool: pool}
}

// Record inserts one usage row for a single LLM call.
func (r *UsageRepository) Record(ctx context.Context, u llm.Usage, cost decimal.Decimal) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO assistant_usage
		   (model, phase, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		u.Model, u.Phase, u.InputTokens, u.OutputTokens, u.CacheReadTokens, u.CacheWriteTokens, cost)
	return err
}

// AggregateMonth sums usage over [start, end) and buckets it by local calendar
// day in tz. InteractionCount counts only "decide" rows so a read interaction
// (decide + summarize = 2 rows) is not double-counted. The caller sets Month and
// Currency on the returned value.
func (r *UsageRepository) AggregateMonth(ctx context.Context, start, end time.Time, tz string) (MonthlyUsage, error) {
	var out MonthlyUsage
	row := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(cost_usd),0),
		        COALESCE(SUM(input_tokens),0),
		        COALESCE(SUM(output_tokens),0),
		        COUNT(*) FILTER (WHERE phase = 'decide')
		   FROM assistant_usage
		  WHERE created_at >= $1 AND created_at < $2`, start, end)
	if err := row.Scan(&out.TotalCostUSD, &out.TotalInputTokens, &out.TotalOutputTokens, &out.InteractionCount); err != nil {
		return MonthlyUsage{}, err
	}

	rows, err := r.pool.Query(ctx,
		`SELECT (created_at AT TIME ZONE $3)::date AS day,
		        COALESCE(SUM(cost_usd),0),
		        COUNT(*) FILTER (WHERE phase = 'decide')
		   FROM assistant_usage
		  WHERE created_at >= $1 AND created_at < $2
		  GROUP BY day
		  ORDER BY day`, start, end, tz)
	if err != nil {
		return MonthlyUsage{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var day time.Time
		var cost decimal.Decimal
		var count int64
		if err := rows.Scan(&day, &cost, &count); err != nil {
			return MonthlyUsage{}, err
		}
		out.ByDay = append(out.ByDay, DayUsage{
			Date:    day.Format("2006-01-02"),
			CostUSD: cost,
			Count:   count,
		})
	}
	if err := rows.Err(); err != nil {
		return MonthlyUsage{}, err
	}
	return out, nil
}
