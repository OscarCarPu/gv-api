package assistant

import (
	"context"
	"testing"
	"time"

	"gv-api/internal/assistant/llm"
	"gv-api/internal/config"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func haikuPrices() map[string]config.ModelPrice {
	return map[string]config.ModelPrice{
		"claude-haiku-4-5": {
			InputPerMTok:      decimal.RequireFromString("1.00"),
			OutputPerMTok:     decimal.RequireFromString("5.00"),
			CacheReadPerMTok:  decimal.RequireFromString("0.10"),
			CacheWritePerMTok: decimal.RequireFromString("1.25"),
		},
	}
}

func TestCost_BasicInputOutput(t *testing.T) {
	// 1000 input * $1/M + 200 output * $5/M = 0.001 + 0.001 = 0.002
	got := Cost(haikuPrices()["claude-haiku-4-5"], llm.Usage{InputTokens: 1000, OutputTokens: 200})
	require.True(t, got.Equal(decimal.RequireFromString("0.002000")), "got %s", got)
}

func TestCost_AllFourBuckets(t *testing.T) {
	// in 1_000_000*1 = 1.0 ; out 1_000_000*5 = 5.0 ; cacheRead 1_000_000*0.10 = 0.10 ;
	// cacheWrite 1_000_000*1.25 = 1.25 ; total = 7.35
	got := Cost(haikuPrices()["claude-haiku-4-5"], llm.Usage{
		InputTokens: 1_000_000, OutputTokens: 1_000_000,
		CacheReadTokens: 1_000_000, CacheWriteTokens: 1_000_000,
	})
	require.True(t, got.Equal(decimal.RequireFromString("7.350000")), "got %s", got)
}

func TestCost_RoundsToSixPlaces(t *testing.T) {
	got := Cost(haikuPrices()["claude-haiku-4-5"], llm.Usage{InputTokens: 1})
	require.Equal(t, int32(-6), got.Exponent(), "cost should be scaled to 6 dp")
}

// capturingRecorder records what meter() persists.
type capturingRecorder struct {
	got  []llm.Usage
	cost []decimal.Decimal
}

func (c *capturingRecorder) Record(_ context.Context, u llm.Usage, cost decimal.Decimal) error {
	c.got = append(c.got, u)
	c.cost = append(c.cost, cost)
	return nil
}
func (c *capturingRecorder) AggregateMonth(context.Context, time.Time, time.Time, string) (MonthlyUsage, error) {
	return MonthlyUsage{}, nil
}

func TestMeter_UnknownModelRecordsZeroCost(t *testing.T) {
	rec := &capturingRecorder{}
	s := &Service{prices: haikuPrices(), usage: rec}
	// A model id not present in the price table must record cost 0, not panic.
	s.meter(context.Background(), llm.Usage{Model: "claude-haiku-4-5-20251001", InputTokens: 1000})
	require.Len(t, rec.cost, 1)
	require.True(t, rec.cost[0].IsZero(), "unknown model should record zero cost, got %s", rec.cost[0])
}

func TestMeter_KnownModelRecordsComputedCost(t *testing.T) {
	rec := &capturingRecorder{}
	s := &Service{prices: haikuPrices(), usage: rec}
	s.meter(context.Background(), llm.Usage{Model: "claude-haiku-4-5", InputTokens: 1000, OutputTokens: 200})
	require.Len(t, rec.cost, 1)
	require.True(t, rec.cost[0].Equal(decimal.RequireFromString("0.002000")), "got %s", rec.cost[0])
}
