package assistant

import (
	_ "embed"
	"regexp"
	"strings"
)

// EmbeddedSchema is the curated, compact schema/context fed to the LLM. It is
// the contract surface (only tables/columns the assistant may reference), not a
// dump of the physical schema. A drift test asserts every column here still
// exists in the database.
//
//go:embed schema.txt
var EmbeddedSchema string

// instructions is the fixed behavioural preamble of the system prompt.
const instructions = `Eres el asistente "Voz" de una app personal (un solo usuario). Conviertes una petición en lenguaje natural en UNA de estas decisiones estructuradas:
- kind="read": una consulta SELECT de solo lectura (PostgreSQL) sobre el esquema de abajo, cuando el usuario quiere consultar/ver/contar/resumir datos. Rellena sql y explanation, y needs_summary=true.
- kind="write": UNA acción estructurada del catálogo ACTIONS, cuando el usuario quiere crear/registrar/modificar algo. Rellena action{domain,operation,args} y explanation.
- kind="reject": cuando la petición es ambigua, imposible, o no encaja en el esquema/acciones. Rellena reject con un motivo breve en español pidiendo reformular.
Reglas: usa SOLO tablas y columnas del esquema. No inventes columnas ni acciones. explanation SIEMPRE en español, claro y breve, describiendo qué hará la consulta o acción. Para escrituras, respeta las RULES (p.ej. category.type == transaction.type). Ante la duda, comprueba primero con consultas internas si tienes esa opción disponible (ver EXPLORACIÓN); si sigue sin estar claro, kind="reject".`

// buildSystemPrompt assembles the stable system prompt: instructions + curated
// schema + the action catalog (generated from the same registry that dispatches
// writes, so the prompt and the dispatch table never diverge).
func buildSystemPrompt(reg *ActionRegistry) string {
	var b strings.Builder
	b.WriteString(instructions)
	b.WriteString("\n\n")
	b.WriteString(EmbeddedSchema)
	b.WriteString("\n")
	if reg != nil {
		b.WriteString(reg.CatalogPrompt())
	}
	return b.String()
}

// ColumnRef is a table.column reference parsed from the schema doc.
type ColumnRef struct {
	Table string
	Col   string
}

// tableLineRe matches an indented "table(col, col:type, ...)" line inside the
// TABLES section. Section headers (non-indented) and RULES lines (starting with
// "-") do not match.
var tableLineRe = regexp.MustCompile(`^\s+([a-z_][a-z0-9_]*)\((.+)\)\s*$`)

// ParseSchemaColumnRefs extracts (table, column) references from the schema doc's
// TABLES section. Column type annotations after ":" are stripped. It checks
// existence, not exhaustiveness — omitting a column from the doc is a deliberate
// curation choice, not drift.
func ParseSchemaColumnRefs(schema string) []ColumnRef {
	var refs []ColumnRef
	inTables := false
	for _, line := range strings.Split(schema, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Section headers are non-indented.
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inTables = strings.HasPrefix(strings.ToUpper(trimmed), "TABLES")
			continue
		}
		if !inTables {
			continue
		}
		m := tableLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		table := m[1]
		for _, col := range strings.Split(m[2], ",") {
			col = strings.TrimSpace(col)
			if idx := strings.IndexByte(col, ':'); idx >= 0 {
				col = strings.TrimSpace(col[:idx])
			}
			if col == "" {
				continue
			}
			refs = append(refs, ColumnRef{Table: table, Col: col})
		}
	}
	return refs
}
