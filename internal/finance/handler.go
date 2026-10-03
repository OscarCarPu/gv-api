package finance

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"gv-api/internal/core"

	"github.com/go-chi/chi/v5"
)

type ServiceInterface interface {
	GetAccount(ctx context.Context, id int32) (Account, error)
	ListAccounts(ctx context.Context) ([]Account, error)
	CreateAccount(ctx context.Context, req CreateAccountRequest) (Account, error)
	UpdateAccount(ctx context.Context, req UpdateAccountRequest) (Account, error)
	DeleteAccount(ctx context.Context, id int32) error

	GetCategory(ctx context.Context, id int32) (Category, error)
	ListCategories(ctx context.Context) ([]Category, error)
	CreateCategory(ctx context.Context, req CreateCategoryRequest) (Category, error)
	UpdateCategory(ctx context.Context, req UpdateCategoryRequest) (Category, error)
	DeleteCategory(ctx context.Context, id int32) error

	GetTransaction(ctx context.Context, id int32) (Transaction, error)
	ListTransactions(ctx context.Context, q ListTransactionsQuery) ([]Transaction, error)
	CreateTransaction(ctx context.Context, req CreateTransactionRequest) (Transaction, error)
	UpdateTransaction(ctx context.Context, req UpdateTransactionRequest) (Transaction, error)
	DeleteTransaction(ctx context.Context, id int32) error

	GetOverview(ctx context.Context) (Overview, error)

	GetNetWorthSeries(ctx context.Context, q NetWorthQuery) ([]NetWorthPoint, error)
	GetCategoryStats(ctx context.Context, q CategoryStatsQuery) ([]CategoryStat, error)
	GetMonthlyStats(ctx context.Context, q MonthlyStatsQuery) ([]MonthlyStat, error)
	GetEstimation(ctx context.Context, q EstimationQuery) (EstimationResult, error)

	GetBudgetMonth(ctx context.Context, month time.Time) (BudgetMonth, error)
	SetBudget(ctx context.Context, req SetBudgetRequest) error
	GetBudgetTransactions(ctx context.Context, categoryID int32, period BudgetPeriod, month time.Time) ([]OverviewTransaction, error)
}

type Handler struct {
	service ServiceInterface
}

func NewHandler(s ServiceInterface) *Handler {
	return &Handler{service: s}
}

// RegisterRoutes mounts all finance endpoints under full auth.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/finance/accounts", h.ListAccounts)
	r.Get("/finance/accounts/{id}", h.GetAccount)
	r.Post("/finance/accounts", h.CreateAccount)
	r.Put("/finance/accounts/{id}", h.UpdateAccount)
	r.Delete("/finance/accounts/{id}", h.DeleteAccount)

	r.Get("/finance/overview", h.GetOverview)

	r.Get("/finance/categories", h.ListCategories)
	r.Get("/finance/categories/{id}", h.GetCategory)
	r.Post("/finance/categories", h.CreateCategory)
	r.Put("/finance/categories/{id}", h.UpdateCategory)
	r.Delete("/finance/categories/{id}", h.DeleteCategory)

	r.Get("/finance/transactions", h.ListTransactions)
	r.Get("/finance/transactions/{id}", h.GetTransaction)
	r.Post("/finance/transactions", h.CreateTransaction)
	r.Put("/finance/transactions/{id}", h.UpdateTransaction)
	r.Delete("/finance/transactions/{id}", h.DeleteTransaction)

	r.Get("/finance/stats/networth", h.GetNetWorthStats)
	r.Get("/finance/stats/by-category", h.GetCategoryStats)
	r.Get("/finance/stats/monthly", h.GetMonthlyStats)
	r.Get("/finance/stats/estimation", h.GetEstimation)

	r.Get("/finance/budgets", h.GetBudgets)
	r.Get("/finance/budgets/{id}/transactions", h.GetBudgetTransactions)
	r.Put("/finance/budgets/{id}", h.SetBudget)
	r.Delete("/finance/budgets/{id}", h.DeleteBudget)
}

func parseDateParam(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, errors.New("invalid date")
}

// parseDateEndParam is like parseDateParam but treats a bare YYYY-MM-DD as the end of that day.
func parseDateEndParam(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Add(24*time.Hour - time.Nanosecond), nil
	}
	return time.Time{}, errors.New("invalid date")
}

func parseOptionalIntParam(s string) (*int32, error) {
	if s == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return nil, errors.New("invalid")
	}
	v := int32(n)
	return &v, nil
}

func parseMonthParam(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("required")
	}
	if t, err := time.Parse("2006-01", s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, errors.New("invalid")
}

func writeTxErr(w http.ResponseWriter, r *http.Request, err error, op string) {
	switch {
	case errors.Is(err, ErrNotFound):
		core.Error(w, http.StatusNotFound, "transaction not found")
	case errors.Is(err, ErrCategoryMismatch):
		core.Error(w, http.StatusBadRequest, "category type does not match transaction type")
	case errors.Is(err, ErrInvalidInput):
		core.Error(w, http.StatusBadRequest, "referenced account or category does not exist")
	default:
		core.InternalError(w, r, err, "Failed to "+op+" transaction")
	}
}
