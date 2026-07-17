package llm

import (
	"context"
	"fmt"
	"strings"
)

// StubProvider is a deterministic, dependency-free Provider used for local
// development and demos when no real LLM is configured (ASSISTANT_PROVIDER=stub).
// It never calls out to a network. Decide always proposes the same safe read
// query so the suggest -> execute -> summarize flow can be exercised end to end;
// Summarize echoes the row count.
type StubProvider struct{}

func NewStubProvider() *StubProvider { return &StubProvider{} }

func (s *StubProvider) Decide(_ context.Context, in DecideInput) (Decision, Usage, error) {
	return Decision{
			Kind:         KindRead,
			SQL:          "SELECT count(*) AS total FROM habits",
			Explanation:  "Proveedor de prueba (stub): cuenta cuántos hábitos hay. Configura un proveedor real para consultas de verdad.",
			NeedsSummary: true,
		},
		Usage{Model: "stub", Phase: PhaseDecide, InputTokens: int64(len(in.UserText)), OutputTokens: 8},
		nil
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
