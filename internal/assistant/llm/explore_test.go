package llm

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// recordingRunner is a QueryRunner that returns a canned result and records what
// the model asked for. Used by the provider tests, which must not touch a database.
type recordingRunner struct {
	result  QueryResult
	queries []string
}

func (r *recordingRunner) RunQuery(_ context.Context, sql string) (QueryResult, error) {
	r.queries = append(r.queries, sql)
	return r.result, nil
}

func TestExploreSystemNote_FormatsCleanly(t *testing.T) {
	// The note carries both a %d budget and a literal SQL wildcard, so a stray
	// escape would silently corrupt the instruction the model reads.
	got := fmt.Sprintf(exploreSystemNote, 5)
	require.Contains(t, got, "Máximo 5 consultas")
	require.Contains(t, got, "ILIKE '%leer%'")
	require.NotContains(t, got, "%!", "unescaped verb in the prompt")
}

func TestParseExploreJSON(t *testing.T) {
	t.Run("explore turn", func(t *testing.T) {
		got := parseExploreJSON(`{"kind":"explore","queries":["SELECT 1"," ","SELECT 2"]}`)
		require.Equal(t, []string{"SELECT 1", "SELECT 2"}, got, "blank queries are dropped")
	})
	t.Run("fenced explore turn", func(t *testing.T) {
		got := parseExploreJSON("```json\n{\"kind\":\"explore\",\"queries\":[\"SELECT 1\"]}\n```")
		require.Equal(t, []string{"SELECT 1"}, got)
	})
	t.Run("decision turn is not exploration", func(t *testing.T) {
		require.Nil(t, parseExploreJSON(`{"kind":"read","sql":"SELECT 1","explanation":"uno"}`))
	})
	t.Run("garbage", func(t *testing.T) {
		require.Nil(t, parseExploreJSON("lo siento, no puedo"))
	})
}

func TestRenderQueryResult(t *testing.T) {
	t.Run("error replaces rows", func(t *testing.T) {
		out := renderQueryResult(QueryResult{Err: "only SELECT/WITH queries are allowed"})
		require.Equal(t, "ERROR: only SELECT/WITH queries are allowed", out)
	})
	t.Run("empty result is stated explicitly", func(t *testing.T) {
		out := renderQueryResult(QueryResult{Columns: []string{"n"}})
		require.Contains(t, out, "filas: 0")
		require.Contains(t, out, "(sin resultados)")
	})
	t.Run("rows are rendered with column names", func(t *testing.T) {
		out := renderQueryResult(QueryResult{Columns: []string{"name", "n"}, Rows: [][]any{{"gym", 3}}})
		require.Contains(t, out, "name=gym, n=3")
	})
	t.Run("oversized payload is capped and stays valid utf-8", func(t *testing.T) {
		rows := make([][]any, 0, 200)
		for i := 0; i < 200; i++ {
			rows = append(rows, []any{strings.Repeat("ñ", 200)})
		}
		out := renderQueryResult(QueryResult{Columns: []string{"txt"}, Rows: rows})
		require.LessOrEqual(t, len(out), exploreRenderChars+len("\n(recortado)\n"))
		require.True(t, strings.ToValidUTF8(out, "") == out, "must not cut mid-rune")
	})
}
