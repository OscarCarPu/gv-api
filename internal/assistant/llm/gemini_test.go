package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

// stubGemini serves canned candidate texts in order and records request bodies.
func stubGemini(t *testing.T, texts ...string) (*GeminiProvider, *[]map[string]any) {
	t.Helper()
	var bodies []map[string]any
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(raw, &body))
		bodies = append(bodies, body)

		require.Less(t, call, len(texts), "provider made more calls than the test expected")
		resp := map[string]any{
			"candidates": []any{map[string]any{
				"content":      map[string]any{"parts": []any{map[string]any{"text": texts[call]}}},
				"finishReason": "STOP",
			}},
			"usageMetadata": map[string]any{"promptTokenCount": 100, "candidatesTokenCount": 20},
		}
		call++
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	p := NewGeminiProvider("test-key", "gemini-3.1-flash-lite")
	p.base = srv.URL
	return p, &bodies
}

func TestGeminiDecide_ExploresThenDecides(t *testing.T) {
	p, bodies := stubGemini(t,
		`{"kind":"explore","queries":["SELECT id, name FROM accounts"]}`,
		`{"kind":"read","sql":"SELECT 1","explanation":"uno","needs_summary":true}`,
	)
	runner := &recordingRunner{result: QueryResult{Columns: []string{"id", "name"}, Rows: [][]any{{1, "Bankinter"}}}}

	res, err := p.Decide(context.Background(), DecideInput{
		SystemPrompt: "SISTEMA", UserText: "cuánto tengo en Bankinter", Runner: runner, MaxQueries: 3,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"SELECT id, name FROM accounts"}, runner.queries)
	require.Equal(t, KindRead, res.Decision.Kind)
	require.Len(t, res.Usages, 2)
	require.Equal(t, PhaseExplore, res.Usages[0].Phase)
	require.Equal(t, PhaseDecide, res.Usages[1].Phase)

	// The second turn replays the model's own request and feeds back the rows.
	contents, _ := json.Marshal((*bodies)[1]["contents"])
	require.Contains(t, string(contents), "Bankinter")
	require.Contains(t, string(contents), `"model"`)
}

func TestGeminiDecide_ExploreIgnoredWithoutRunner(t *testing.T) {
	// Without a runner the exploration protocol is never offered, so an "explore"
	// reply is a malformed decision, not a query request.
	p, bodies := stubGemini(t, `{"kind":"explore","queries":["SELECT 1"]}`)

	res, err := p.Decide(context.Background(), DecideInput{SystemPrompt: "SISTEMA", UserText: "x"})
	require.NoError(t, err)
	require.Equal(t, "explore", res.Decision.Kind, "passed through; the service treats unknown kinds as reject")
	require.Len(t, res.Usages, 1)

	sys, _ := json.Marshal((*bodies)[0]["systemInstruction"])
	require.NotContains(t, string(sys), "EXPLORACIÓN")
}

func TestGeminiDecide_BudgetForcesDecision(t *testing.T) {
	// A model that keeps asking to explore gets its reply read as the final
	// decision once the budget is spent — the loop cannot spin.
	p, _ := stubGemini(t,
		`{"kind":"explore","queries":["SELECT 1"]}`,
		`{"kind":"read","sql":"SELECT 2","explanation":"dos"}`,
	)
	runner := &recordingRunner{result: QueryResult{Columns: []string{"n"}, Rows: [][]any{{1}}}}

	res, err := p.Decide(context.Background(), DecideInput{UserText: "x", Runner: runner, MaxQueries: 1})
	require.NoError(t, err)
	require.Len(t, runner.queries, 1)
	require.Equal(t, "SELECT 2", res.Decision.SQL)
}
