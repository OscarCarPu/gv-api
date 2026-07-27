package llm

import (
	"fmt"
	"strings"
)

// Caps on what one internal read may put back into the prompt.
//
// The loop itself needs no separate round cap: DecideInput.MaxQueries is the only
// bound, because a round that explores always spends at least one query, so the
// model gets at most MaxQueries exploratory turns plus the final deciding one.
const (
	exploreRenderRows  = 30
	exploreRenderChars = 4000
)

// exploreSystemNote is appended to the system prompt when exploration is
// enabled. %d is the query budget.
const exploreSystemNote = `EXPLORACIÓN (antes de decidir)
Antes de emitir la decisión final puedes ejecutar consultas SELECT de solo lectura para informarte: encontrar el nombre o id exacto de una cuenta/categoría/tarea/hábito, ver qué datos existen y en qué rango de fechas, distinguir entre varias interpretaciones posibles, o comprobar que tu consulta final devuelve algo. Son consultas internas y automáticas: no las ve ni las aprueba el usuario, y no pueden modificar datos.
Cuándo DEBES explorar:
- Si la petición nombra una entidad (cuenta, categoría, tarea, proyecto, hábito, concello, variedad) y no has visto ya cómo se llama exactamente en la base de datos: lístalas antes de decidir SIN filtrar por nombre (p.ej. SELECT id, name FROM habits). Son tablas pequeñas, así ves todos los nombres de una vez, y un WHERE name ILIKE '%%leer%%' devolvería 0 filas aunque la entidad exista escrita de otra forma o en otro idioma ("leer" puede estar guardado como "Reading").
  Con la lista delante: si hay una candidata claramente equivalente, úsala (mejor por id). Rechaza solo si ninguna encaja, y entonces nombra en reject las que sí existen para que el usuario elija.
- Si la petición admite varias lecturas y los datos pueden desambiguarla.
- Si dudas de si tu consulta final devuelve algo.
No explores lo que ya sabes. Máximo %d consultas en total.
Si una consulta falla te devolveré el error: corrígela y reintenta.
Cuando ya lo tengas claro, emite la decisión final (read/write/reject).`

// budgetExhaustedNote is fed back to the model in place of a query result once
// the budget is spent, so it stops asking and commits to a decision.
const budgetExhaustedNote = "ERROR: presupuesto de consultas internas agotado. Emite ya la decisión final con lo que sabes."

// exploreEnvelope is the JSON shape a text-only provider (Gemini) uses to ask
// for internal reads instead of returning a decision.
type exploreEnvelope struct {
	Kind    string   `json:"kind"`
	Queries []string `json:"queries"`
}

// renderQueryResult formats one internal read's outcome for the model. Row and
// character caps keep an accidental wide/long result from swamping the context.
func renderQueryResult(r QueryResult) string {
	if r.Err != "" {
		return "ERROR: " + r.Err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "columnas: %s\nfilas: %d\n", strings.Join(r.Columns, ", "), len(r.Rows))
	if len(r.Rows) == 0 {
		b.WriteString("(sin resultados)\n")
	} else {
		b.WriteString(renderRows(r.Columns, r.Rows, exploreRenderRows))
	}
	if r.Truncated {
		b.WriteString("(resultado truncado por el límite de filas)\n")
	}
	s := b.String()
	if len(s) > exploreRenderChars {
		// Cut on a byte boundary, then drop any partial rune at the end.
		s = strings.ToValidUTF8(s[:exploreRenderChars], "") + "\n(recortado)\n"
	}
	return s
}
