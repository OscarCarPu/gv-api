package finance

import "errors"

// Sentinel errors for the finance domain.
var (
	ErrNotFound         = errors.New("not found")
	ErrAccountInUse     = errors.New("account has transactions")
	ErrCategoryInUse    = errors.New("category is referenced")
	ErrInvalidInput     = errors.New("invalid input")
	ErrCategoryMismatch = errors.New("category type does not match transaction type")
)
