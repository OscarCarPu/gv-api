package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"
)

// stubAnthropic serves the given canned responses in order and records every
// request body, so the tool loop can be exercised without a network.
func stubAnthropic(t *testing.T, responses ...string) (*AnthropicProvider, *[]map[string]any) {
	t.Helper()
	var bodies []map[string]any
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var body map[string]any
		require.NoError(t, json.Unmarshal(raw, &body))
		bodies = append(bodies, body)

		require.Less(t, call, len(responses), "provider made more calls than the test expected")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responses[call]))
		call++
	}))
	t.Cleanup(srv.Close)
	return NewAnthropicProvider("test-key", "claude-haiku-4-5", option.WithBaseURL(srv.URL)), &bodies
}

func toolUseResponse(id, name, input string) string {
	return `{"id":"msg_x","type":"message","role":"assistant","model":"claude-haiku-4-5",
	  "content":[{"type":"tool_use","id":"` + id + `","name":"` + name + `","input":` + input + `}],
	  "stop_reason":"tool_use","usage":{"input_tokens":100,"output_tokens":20}}`
}

// toolNames returns the tool names offered in a recorded request body.
func toolNames(t *testing.T, body map[string]any) []string {
	t.Helper()
	raw, ok := body["tools"].([]any)
	require.True(t, ok, "request has no tools")
	names := make([]string, 0, len(raw))
	for _, entry := range raw {
		tool, ok := entry.(map[string]any)
		require.True(t, ok)
		names = append(names, tool["name"].(string))
	}
	return names
}

func TestAnthropicDecide_ExploresThenDecides(t *testing.T) {
	p, bodies := stubAnthropic(t,
		toolUseResponse("toolu_1", "run_read_query", `{"sql":"SELECT id, name FROM accounts","reason":"encontrar la cuenta"}`),
		toolUseResponse("toolu_2", "emit_decision", `{"kind":"read","sql":"SELECT 1","explanation":"uno","needs_summary":true}`),
	)
	runner := &recordingRunner{result: QueryResult{Columns: []string{"id", "name"}, Rows: [][]any{{1, "Bankinter"}}}}

	res, err := p.Decide(context.Background(), DecideInput{
		SystemPrompt: "SISTEMA",
		UserText:     "cuánto tengo en Bankinter",
		Runner:       runner,
		MaxQueries:   3,
	})
	require.NoError(t, err)

	require.Equal(t, []string{"SELECT id, name FROM accounts"}, runner.queries)
	require.Equal(t, KindRead, res.Decision.Kind)
	require.Equal(t, "SELECT 1", res.Decision.SQL)

	// One usage per model call: the exploratory round, then the deciding round.
	require.Len(t, res.Usages, 2)
	require.Equal(t, PhaseExplore, res.Usages[0].Phase)
	require.Equal(t, PhaseDecide, res.Usages[1].Phase)
	require.Equal(t, "claude-haiku-4-5", res.Usages[0].Model)

	require.Len(t, *bodies, 2)
	first, second := (*bodies)[0], (*bodies)[1]

	// The query tool is offered and the exploration protocol is in the system prompt.
	require.ElementsMatch(t, []string{"emit_decision", "run_read_query"}, toolNames(t, first))
	sys, _ := json.Marshal(first["system"])
	require.Contains(t, string(sys), "EXPLORACIÓN")

	// The second request carries the conversation plus the tool_result, and the
	// rows we returned are visible to the model.
	msgs, _ := json.Marshal(second["messages"])
	require.Contains(t, string(msgs), "tool_result")
	require.Contains(t, string(msgs), "toolu_1")
	require.Contains(t, string(msgs), "Bankinter")
}

func TestAnthropicDecide_NoRunnerOffersOnlyDecisionTool(t *testing.T) {
	p, bodies := stubAnthropic(t,
		toolUseResponse("toolu_1", "emit_decision", `{"kind":"write","explanation":"crea","action":{"domain":"tasks","operation":"create_task","args":{"name":"x"}}}`),
	)

	res, err := p.Decide(context.Background(), DecideInput{SystemPrompt: "SISTEMA", UserText: "crea tarea x"})
	require.NoError(t, err)
	require.Equal(t, KindWrite, res.Decision.Kind)
	require.Len(t, res.Usages, 1)
	require.Equal(t, PhaseDecide, res.Usages[0].Phase)

	require.Equal(t, []string{"emit_decision"}, toolNames(t, (*bodies)[0]))
	sys, _ := json.Marshal((*bodies)[0]["system"])
	require.NotContains(t, string(sys), "EXPLORACIÓN")
}

func TestAnthropicDecide_ParallelQueriesAllAnswered(t *testing.T) {
	// Two tool_use blocks in one turn must each get a matching tool_result, or the
	// API rejects the follow-up request.
	twoQueries := `{"id":"msg_x","type":"message","role":"assistant","model":"claude-haiku-4-5",
	  "content":[
	    {"type":"tool_use","id":"toolu_a","name":"run_read_query","input":{"sql":"SELECT 1"}},
	    {"type":"tool_use","id":"toolu_b","name":"run_read_query","input":{"sql":"SELECT 2"}}],
	  "stop_reason":"tool_use","usage":{"input_tokens":10,"output_tokens":5}}`
	p, bodies := stubAnthropic(t, twoQueries,
		toolUseResponse("toolu_c", "emit_decision", `{"kind":"read","sql":"SELECT 3","explanation":"tres"}`),
	)
	runner := &recordingRunner{result: QueryResult{Columns: []string{"n"}, Rows: [][]any{{1}}}}

	res, err := p.Decide(context.Background(), DecideInput{UserText: "x", Runner: runner, MaxQueries: 3})
	require.NoError(t, err)
	require.Equal(t, []string{"SELECT 1", "SELECT 2"}, runner.queries)
	require.Equal(t, "SELECT 3", res.Decision.SQL)

	msgs, _ := json.Marshal((*bodies)[1]["messages"])
	require.Equal(t, 2, strings.Count(string(msgs), `"tool_result"`), "one result per tool_use")
}

func TestAnthropicDecide_BudgetForcesDecision(t *testing.T) {
	// With a budget of one query, the second round must force emit_decision so the
	// loop cannot spin.
	p, bodies := stubAnthropic(t,
		toolUseResponse("toolu_1", "run_read_query", `{"sql":"SELECT 1"}`),
		toolUseResponse("toolu_2", "emit_decision", `{"kind":"reject","explanation":"no","reject":"no"}`),
	)
	runner := &recordingRunner{result: QueryResult{Columns: []string{"n"}, Rows: [][]any{{1}}}}

	res, err := p.Decide(context.Background(), DecideInput{UserText: "x", Runner: runner, MaxQueries: 1})
	require.NoError(t, err)
	require.Equal(t, KindReject, res.Decision.Kind)
	require.Len(t, runner.queries, 1)

	choice, _ := json.Marshal((*bodies)[1]["tool_choice"])
	require.Contains(t, string(choice), "emit_decision")
}

func TestAnthropicDecide_OneQueryPerRoundStillTerminates(t *testing.T) {
	// The budget is the loop's only bound, which holds because every exploring
	// round spends at least one query. A model that drips one query per round gets
	// exactly MaxQueries exploratory calls plus the forced deciding call — the
	// stub fails the test if the provider asks for more than these 4.
	p, bodies := stubAnthropic(t,
		toolUseResponse("toolu_1", "run_read_query", `{"sql":"SELECT 1"}`),
		toolUseResponse("toolu_2", "run_read_query", `{"sql":"SELECT 2"}`),
		toolUseResponse("toolu_3", "run_read_query", `{"sql":"SELECT 3"}`),
		toolUseResponse("toolu_4", "emit_decision", `{"kind":"read","sql":"SELECT 4","explanation":"cuatro"}`),
	)
	runner := &recordingRunner{result: QueryResult{Columns: []string{"n"}, Rows: [][]any{{1}}}}

	res, err := p.Decide(context.Background(), DecideInput{UserText: "x", Runner: runner, MaxQueries: 3})
	require.NoError(t, err)
	require.Len(t, runner.queries, 3)
	require.Equal(t, "SELECT 4", res.Decision.SQL)
	require.Len(t, res.Usages, 4)
	require.Len(t, *bodies, 4)

	// The deciding call is forced; the exploratory ones leave the choice open.
	last, _ := json.Marshal((*bodies)[3]["tool_choice"])
	require.Contains(t, string(last), "emit_decision")
	first, _ := json.Marshal((*bodies)[0]["tool_choice"])
	require.NotContains(t, string(first), "emit_decision")
}

func TestAnthropicDecide_TextOnlyReplyRejects(t *testing.T) {
	textOnly := `{"id":"msg_x","type":"message","role":"assistant","model":"claude-haiku-4-5",
	  "content":[{"type":"text","text":"lo siento"}],
	  "stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`
	p, _ := stubAnthropic(t, textOnly)

	res, err := p.Decide(context.Background(), DecideInput{UserText: "x"})
	require.NoError(t, err)
	require.Equal(t, KindReject, res.Decision.Kind)
	require.Len(t, res.Usages, 1, "the wasted call is still metered")
}
