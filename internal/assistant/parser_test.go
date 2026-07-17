package assistant

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func hasRef(refs []ColumnRef, table, col string) bool {
	for _, r := range refs {
		if r.Table == table && r.Col == col {
			return true
		}
	}
	return false
}

func TestParseSchemaColumnRefs_EmbeddedSchema(t *testing.T) {
	refs := ParseSchemaColumnRefs(EmbeddedSchema)
	require.NotEmpty(t, refs)

	// Real columns are extracted.
	require.True(t, hasRef(refs, "transactions", "occurred_at"))
	require.True(t, hasRef(refs, "habit_logs", "log_date"))
	require.True(t, hasRef(refs, "accounts", "total"))
	require.True(t, hasRef(refs, "concello_marks", "visited_on"))

	// Section headers and RULES/ENUMS lines must NOT produce bogus refs.
	for _, r := range refs {
		require.NotEqual(t, "tables", r.Table)
		require.NotEqual(t, "rules", r.Table)
		require.NotEqual(t, "enums", r.Table)
		require.NotContains(t, r.Col, "|")
		require.NotContains(t, r.Col, "-")
		require.NotContains(t, r.Col, " ")
	}
}

func TestParseSchemaColumnRefs_StripsTypeAnnotations(t *testing.T) {
	schema := "TABLES:\n  foo(id, amount:decimal, kind:enum)\n\nRULES:\n  - bar(baz) should be ignored\n"
	refs := ParseSchemaColumnRefs(schema)
	require.True(t, hasRef(refs, "foo", "id"))
	require.True(t, hasRef(refs, "foo", "amount"))
	require.True(t, hasRef(refs, "foo", "kind"))
	// The RULES line "bar(baz)" is outside TABLES and must be ignored.
	require.False(t, hasRef(refs, "bar", "baz"))
}
