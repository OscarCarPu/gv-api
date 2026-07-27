package llm

import (
	"context"
	"fmt"
	"strings"
)

// StubProvider is a deterministic, dependency-free Provider used for local
// development and demos when no real LLM is configured (ASSISTANT_PROVIDER=stub).
// It never calls out to a network. Decide runs one internal read (so the
// exploration loop is exercised locally) and then always proposes the same safe
// read query, so suggest -> execute -> summarize works end to end; Summarize
// echoes the row count.
type StubProvider struct{}

func NewStubProvider() *StubProvider { return &StubProvider{} }

func (s *StubProvider) Decide(ctx context.Context, in DecideInput) (DecideResult, error) {
	var out DecideResult

	if in.Runner != nil && in.MaxQueries > 0 {
		if _, err := in.Runner.RunQuery(ctx, "SELECT count(*) AS total FROM habits"); err != nil {
			return out, err
		}
		out.Usages = append(out.Usages, Usage{Model: "stub", Phase: PhaseExplore, InputTokens: 8, OutputTokens: 8})
	}

	out.Decision = Decision{
		Kind:         KindRead,
		SQL:          "SELECT count(*) AS total FROM habits",
		Explanation:  "Proveedor de prueba (stub): cuenta cuántos hábitos hay. Configura un proveedor real para consultas de verdad.",
		NeedsSummary: true,
	}
	out.Usages = append(out.Usages, Usage{Model: "stub", Phase: PhaseDecide, InputTokens: int64(len(in.UserText)), OutputTokens: 8})
	return out, nil
}

func (s *StubProvider) Summarize(_ context.Context, in SummarizeInput) (string, Usage, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "La consulta devolvió %d fila(s).", len(in.Rows))
	if in.Truncated {
		b.WriteString(" (resultado truncado)")
	}
	return b.String(),
		Usage{Model: "stub", Phase: PhaseSummarize, InputTokens: int64(len(in.Rows)), OutputTokens: 12},
		nil
}
