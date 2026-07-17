package assistant

import (
	"gv-api/internal/assistant/llm"
	"gv-api/internal/config"

	"github.com/shopspring/decimal"
)

var millionDec = decimal.NewFromInt(1_000_000)

// Cost computes the USD cost of one LLM call from its four (mutually exclusive)
// token buckets and the model's price table. The SDK's InputTokens is already
// the uncached remainder, so summing the buckets is exact. Result is rounded to
// 6 decimal places to match the NUMERIC(12,6) storage column.
func Cost(p config.ModelPrice, u llm.Usage) decimal.Decimal {
	perM := func(tokens int64, price decimal.Decimal) decimal.Decimal {
		return decimal.NewFromInt(tokens).Mul(price)
	}
	return perM(u.InputTokens, p.InputPerMTok).
		Add(perM(u.OutputTokens, p.OutputPerMTok)).
		Add(perM(u.CacheReadTokens, p.CacheReadPerMTok)).
		Add(perM(u.CacheWriteTokens, p.CacheWritePerMTok)).
		Div(millionDec).
		Round(6)
}
