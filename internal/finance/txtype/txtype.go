// Package txtype defines the Type enum mapped to Postgres `transaction_type`. A separate
// package so sqlc-generated gvdb can import it without a cycle.
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
