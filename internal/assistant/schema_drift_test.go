package assistant

import (
	"context"
	"testing"

	"gv-api/internal/testutil"

	"github.com/stretchr/testify/require"
)

// TestSchemaDoc_Integration_ColumnsExist guards the embedded LLM schema doc
// against drift: every table.column it references must still exist in the
// migrated database. It checks existence, not exhaustiveness — omitting a column
// from the doc is deliberate curation, not drift.
func TestSchemaDoc_Integration_ColumnsExist(t *testing.T) {
	pool := testutil.NewPool(t)
	ctx := context.Background()

	refs := ParseSchemaColumnRefs(EmbeddedSchema)
	require.NotEmpty(t, refs)

	for _, ref := range refs {
		var exists bool
		err := pool.QueryRow(ctx,
			`SELECT EXISTS (
			   SELECT 1 FROM information_schema.columns
			   WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
			 )`, ref.Table, ref.Col).Scan(&exists)
		require.NoError(t, err)
		require.Truef(t, exists, "schema.txt references %s.%s which does not exist in the database", ref.Table, ref.Col)
	}
}
