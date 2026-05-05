// Package txtype defines the Type enum that maps to the Postgres
// `transaction_type` enum on both `transactions.type` and `categories.type`.
//
// It lives in its own package so the sqlc-generated `financedb` package can
// import it (via the sqlc override) without creating a cycle with the
// higher-level `finance` package that consumes financedb.
package txtype

type Type string

const (
	Income   Type = "income"
	Expense  Type = "expense"
	Transfer Type = "transfer"
)

func (t Type) Valid() bool {
	switch t {
	case Income, Expense, Transfer:
		return true
	}
	return false
}
