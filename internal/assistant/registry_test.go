package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"gv-api/internal/assistant/llm"
	"gv-api/internal/finance"
	"gv-api/internal/finance/txtype"
	"gv-api/internal/habits"
	"gv-api/internal/plan"
	"gv-api/internal/rutas"
	"gv-api/internal/tasks"
	"gv-api/internal/varieties"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

// --- fakes (implement the registry's per-domain service interfaces) ---

type fakeTaskService struct {
	last       tasks.CreateTaskRequest
	lastUpdate tasks.UpdateTaskRequest
	deletedID  int32
	err        error
}

func (f *fakeTaskService) CreateTask(_ context.Context, req tasks.CreateTaskRequest) (tasks.TaskResponse, error) {
	f.last = req
	if f.err != nil {
		return tasks.TaskResponse{}, f.err
	}
	return tasks.TaskResponse{ID: 7, Name: req.Name}, nil
}
func (f *fakeTaskService) CreateProject(_ context.Context, req tasks.CreateProjectRequest) (tasks.ProjectResponse, error) {
	return tasks.ProjectResponse{ID: 5, Name: req.Name}, nil
}
func (f *fakeTaskService) CreateTodo(_ context.Context, req tasks.CreateTodoRequest) (tasks.TodoResponse, error) {
	return tasks.TodoResponse{ID: 9, TaskID: req.TaskID, Name: req.Name}, nil
}
func (f *fakeTaskService) UpdateTask(_ context.Context, req tasks.UpdateTaskRequest) (tasks.TaskResponse, error) {
	f.lastUpdate = req
	return tasks.TaskResponse{ID: req.ID}, nil
}
func (f *fakeTaskService) UpdateProject(_ context.Context, req tasks.UpdateProjectRequest) (tasks.ProjectResponse, error) {
	return tasks.ProjectResponse{ID: req.ID}, nil
}
func (f *fakeTaskService) DeleteTask(_ context.Context, id int32) error { f.deletedID = id; return nil }
func (f *fakeTaskService) DeleteProject(_ context.Context, id int32) error {
	f.deletedID = id
	return nil
}
func (f *fakeTaskService) ListOpenTasks(_ context.Context) ([]tasks.TaskFastResponse, error) {
	return []tasks.TaskFastResponse{{ID: 1, Name: "Comprar pan"}, {ID: 2, Name: "Llamar fontanero"}}, nil
}
func (f *fakeTaskService) ListOpenProjects(_ context.Context) ([]tasks.ProjectFastResponse, error) {
	return []tasks.ProjectFastResponse{{ID: 10, Name: "Casa"}, {ID: 11, Name: "Reformas"}}, nil
}

type fakeHabitService struct{ last habits.LogUpsertRequest }

func (f *fakeHabitService) LogHabit(_ context.Context, req habits.LogUpsertRequest) error {
	f.last = req
	return nil
}
func (f *fakeHabitService) CreateHabit(_ context.Context, req habits.CreateHabitRequest) (habits.CreateHabitResponse, error) {
	return habits.CreateHabitResponse{ID: 3, Name: req.Name}, nil
}
func (f *fakeHabitService) UpdateHabit(_ context.Context, req habits.UpdateHabitRequest) (habits.CreateHabitResponse, error) {
	return habits.CreateHabitResponse{ID: req.ID, Name: req.Name}, nil
}
func (f *fakeHabitService) DeleteHabit(_ context.Context, _ int32) error { return nil }
func (f *fakeHabitService) GetDailyView(_ context.Context, _ string) ([]habits.HabitWithLog, error) {
	return []habits.HabitWithLog{{ID: 4, Name: "Correr", Frequency: "daily"}, {ID: 5, Name: "Leer", Frequency: "daily"}}, nil
}

type fakeFinanceService struct {
	last finance.CreateTransactionRequest
}

func (f *fakeFinanceService) CreateTransaction(_ context.Context, req finance.CreateTransactionRequest) (finance.Transaction, error) {
	f.last = req
	return finance.Transaction{ID: 3, Type: req.Type, Amount: req.Amount}, nil
}
func (f *fakeFinanceService) CreateAccount(_ context.Context, req finance.CreateAccountRequest) (finance.Account, error) {
	return finance.Account{ID: 2, Name: req.Name}, nil
}
func (f *fakeFinanceService) CreateCategory(_ context.Context, req finance.CreateCategoryRequest) (finance.Category, error) {
	return finance.Category{ID: 6, Name: req.Name, Type: req.Type}, nil
}
func (f *fakeFinanceService) UpdateTransaction(_ context.Context, req finance.UpdateTransactionRequest) (finance.Transaction, error) {
	return finance.Transaction{ID: req.ID, Type: req.Type, Amount: req.Amount}, nil
}
func (f *fakeFinanceService) UpdateAccount(_ context.Context, req finance.UpdateAccountRequest) (finance.Account, error) {
	return finance.Account{ID: req.ID, Name: req.Name}, nil
}
func (f *fakeFinanceService) UpdateCategory(_ context.Context, req finance.UpdateCategoryRequest) (finance.Category, error) {
	return finance.Category{ID: req.ID, Name: req.Name, Type: req.Type}, nil
}
func (f *fakeFinanceService) GetTransaction(_ context.Context, id int32) (finance.Transaction, error) {
	return finance.Transaction{ID: id, Type: txtype.Expense, Amount: decimal.RequireFromString("10"), AccountID: 1}, nil
}
func (f *fakeFinanceService) DeleteTransaction(_ context.Context, _ int32) error { return nil }
func (f *fakeFinanceService) DeleteAccount(_ context.Context, _ int32) error     { return nil }
func (f *fakeFinanceService) DeleteCategory(_ context.Context, _ int32) error    { return nil }
func (f *fakeFinanceService) ListAccounts(_ context.Context) ([]finance.Account, error) {
	return []finance.Account{{ID: 1, Name: "Cuenta principal"}, {ID: 2, Name: "Ahorro"}}, nil
}
func (f *fakeFinanceService) ListCategories(_ context.Context) ([]finance.Category, error) {
	return []finance.Category{{ID: 8, Name: "Comida", Type: txtype.Expense}}, nil
}

type fakePlanService struct{}

func (f *fakePlanService) Create(_ context.Context, req plan.CreatePlanBlockRequest) (plan.PlanBlockResponse, error) {
	label := ""
	if req.Label != nil {
		label = *req.Label
	}
	return plan.PlanBlockResponse{ID: 1, Label: label}, nil
}
func (f *fakePlanService) Update(_ context.Context, req plan.UpdatePlanBlockRequest) (plan.PlanBlockResponse, error) {
	return plan.PlanBlockResponse{ID: req.ID}, nil
}
func (f *fakePlanService) Delete(_ context.Context, _ int32) error { return nil }

type fakeRutasService struct{ last rutas.CreateMarkRequest }

func (f *fakeRutasService) Create(_ context.Context, req rutas.CreateMarkRequest) (rutas.ConcelloMark, error) {
	f.last = req
	return rutas.ConcelloMark{ID: 1, Name: req.Name}, nil
}
func (f *fakeRutasService) Update(_ context.Context, req rutas.UpdateMarkRequest) (rutas.ConcelloMark, error) {
	return rutas.ConcelloMark{ID: req.ID}, nil
}
func (f *fakeRutasService) Delete(_ context.Context, _ int32) error { return nil }
func (f *fakeRutasService) List(_ context.Context) ([]rutas.ConcelloMark, error) {
	return []rutas.ConcelloMark{{ID: 1, Name: "Santiago"}, {ID: 2, Name: "A Coruña"}}, nil
}

type fakeVarietyService struct{}

func (f *fakeVarietyService) Create(_ context.Context, req varieties.CreateVarietyRequest) (varieties.Variety, error) {
	return varieties.Variety{ID: 1, Name: req.Name, Score: 5}, nil
}
func (f *fakeVarietyService) Update(_ context.Context, req varieties.UpdateVarietyRequest) (varieties.Variety, error) {
	return varieties.Variety{ID: req.ID, Name: req.Name}, nil
}
func (f *fakeVarietyService) Delete(_ context.Context, _ int32) error { return nil }
func (f *fakeVarietyService) List(_ context.Context) ([]varieties.Variety, error) {
	return []varieties.Variety{{ID: 1, Name: "White Widow"}, {ID: 2, Name: "Amnesia"}}, nil
}

func newTestRegistry() (*ActionRegistry, *fakeTaskService, *fakeFinanceService) {
	tw := &fakeTaskService{}
	fw := &fakeFinanceService{}
	return NewActionRegistry(tw, &fakeHabitService{}, fw, &fakePlanService{}, &fakeRutasService{}, &fakeVarietyService{}), tw, fw
}

func call(domain, op, args string) *llm.ActionCall {
	return &llm.ActionCall{Domain: domain, Operation: op, Args: json.RawMessage(args)}
}

func TestRegistry_CreateTask_Valid(t *testing.T) {
	reg, tw, _ := newTestRegistry()
	msg, err := reg.Execute(context.Background(), call("tasks", "create_task", `{"name":"Llamar fontanero","due_at":"2026-07-20","priority":2}`))
	require.NoError(t, err)
	require.Contains(t, msg, "Llamar fontanero")
	require.Equal(t, "Llamar fontanero", tw.last.Name)
	require.NotNil(t, tw.last.DueAt)
	require.NotNil(t, tw.last.Priority)
	require.Equal(t, int32(2), *tw.last.Priority)
}

func TestRegistry_CreateTask_EmptyName(t *testing.T) {
	reg, _, _ := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("tasks", "create_task", `{"name":"  "}`))
	require.ErrorIs(t, err, ErrInvalidAction)
}

func TestRegistry_CreateTask_BadPriority(t *testing.T) {
	reg, _, _ := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("tasks", "create_task", `{"name":"x","priority":9}`))
	require.ErrorIs(t, err, ErrInvalidAction)
}

func TestRegistry_CreateTask_UnknownField(t *testing.T) {
	reg, _, _ := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("tasks", "create_task", `{"name":"x","nope":true}`))
	require.ErrorIs(t, err, ErrInvalidAction)
}

func TestRegistry_UnknownAction(t *testing.T) {
	reg, _, _ := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("tasks", "delete_everything", `{}`))
	require.ErrorIs(t, err, ErrUnsupportedAction)
}

func TestRegistry_Transaction_TransferRequiresToAccount(t *testing.T) {
	reg, _, _ := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("finance", "create_transaction",
		`{"type":"transfer","amount":"10","account_id":1,"category_id":2}`))
	require.ErrorIs(t, err, ErrInvalidAction)
}

func TestRegistry_Transaction_ExpenseRejectsToAccount(t *testing.T) {
	reg, _, _ := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("finance", "create_transaction",
		`{"type":"expense","amount":"10","account_id":1,"category_id":2,"to_account_id":3}`))
	require.ErrorIs(t, err, ErrInvalidAction)
}

func TestRegistry_Transaction_ValidExpense(t *testing.T) {
	reg, _, fw := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("finance", "create_transaction",
		`{"type":"expense","amount":"12.50","account_id":1,"category_id":2,"description":"comida"}`))
	require.NoError(t, err)
	require.Equal(t, int32(1), fw.last.AccountID)
	require.True(t, fw.last.Amount.Equal(decimal.RequireFromString("12.50")))
}

func TestRegistry_Transaction_AmountMustBePositive(t *testing.T) {
	reg, _, _ := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("finance", "create_transaction",
		`{"type":"expense","amount":"0","account_id":1,"category_id":2}`))
	require.ErrorIs(t, err, ErrInvalidAction)
}

func TestRegistry_LogHabit_DefaultsDay(t *testing.T) {
	reg, _, _ := newTestRegistry()
	msg, err := reg.Execute(context.Background(), call("habits", "log_habit", `{"habit_id":4}`))
	require.NoError(t, err)
	require.Contains(t, msg, "registrado")
}

func TestRegistry_CatalogPrompt_ListsActions(t *testing.T) {
	reg, _, _ := newTestRegistry()
	cat := reg.CatalogPrompt()
	require.Contains(t, cat, "tasks.create_task")
	require.Contains(t, cat, "finance.create_transaction")
	require.Contains(t, cat, "habits.log_habit")
	require.Contains(t, cat, "rutas.create_mark")
}

func TestRegistry_NilAction(t *testing.T) {
	reg, _, _ := newTestRegistry()
	_, err := reg.Execute(context.Background(), nil)
	require.True(t, errors.Is(err, ErrInvalidAction))
}

func TestRegistry_DeleteTask_ByName(t *testing.T) {
	reg, tw, _ := newTestRegistry()
	msg, err := reg.Execute(context.Background(), call("tasks", "delete_task", `{"task":"comprar pan"}`))
	require.NoError(t, err)
	require.Equal(t, int32(1), tw.deletedID)
	require.Contains(t, msg, "borrada")
}

func TestRegistry_DeleteTask_ByPartialName(t *testing.T) {
	reg, tw, _ := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("tasks", "delete_task", `{"task":"fontanero"}`))
	require.NoError(t, err)
	require.Equal(t, int32(2), tw.deletedID)
}

func TestRegistry_DeleteTask_NotFound(t *testing.T) {
	reg, _, _ := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("tasks", "delete_task", `{"task":"no existe"}`))
	require.ErrorIs(t, err, ErrInvalidAction)
}

func TestRegistry_CompleteTask_ByName(t *testing.T) {
	reg, tw, _ := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("tasks", "complete_task", `{"task":"Llamar fontanero"}`))
	require.NoError(t, err)
	require.Equal(t, int32(2), tw.lastUpdate.ID)
	require.True(t, tw.lastUpdate.FinishedAt.Set)
	require.NotNil(t, tw.lastUpdate.FinishedAt.Value)
}

func TestRegistry_CreateTransaction_ResolvesAccountAndCategoryByName(t *testing.T) {
	reg, _, fw := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("finance", "create_transaction",
		`{"type":"expense","amount":"5.00","account":"Cuenta principal","category":"Comida"}`))
	require.NoError(t, err)
	require.Equal(t, int32(1), fw.last.AccountID)
	require.NotNil(t, fw.last.CategoryID)
	require.Equal(t, int32(8), *fw.last.CategoryID)
}

func TestRegistry_CreateTransaction_UnknownAccount(t *testing.T) {
	reg, _, _ := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("finance", "create_transaction",
		`{"type":"expense","amount":"5","account":"Cuenta que no existe","category":"Comida"}`))
	require.ErrorIs(t, err, ErrInvalidAction)
}

func TestRegistry_CreateHabit(t *testing.T) {
	reg, _, _ := newTestRegistry()
	msg, err := reg.Execute(context.Background(), call("habits", "create_habit", `{"name":"Meditar","frequency":"daily"}`))
	require.NoError(t, err)
	require.Contains(t, msg, "Meditar")
}

func TestRegistry_LogHabit_ByName(t *testing.T) {
	reg, _, _ := newTestRegistry()
	msg, err := reg.Execute(context.Background(), call("habits", "log_habit", `{"habit":"Correr"}`))
	require.NoError(t, err)
	require.Contains(t, msg, "registrado")
}

func TestRegistry_DeleteMark_ByName(t *testing.T) {
	reg, _, _ := newTestRegistry()
	msg, err := reg.Execute(context.Background(), call("rutas", "delete_mark", `{"mark":"Santiago"}`))
	require.NoError(t, err)
	require.Contains(t, msg, "borrada")
}

func TestRegistry_CreatePlanBlock(t *testing.T) {
	reg, _, _ := newTestRegistry()
	msg, err := reg.Execute(context.Background(), call("plan", "create_block",
		`{"started_at":"2026-07-20 09:00","ended_at":"2026-07-20 10:00","label":"Deep work"}`))
	require.NoError(t, err)
	require.Contains(t, msg, "Deep work")
}

func TestRegistry_CreatePlanBlock_BadTimeOrder(t *testing.T) {
	reg, _, _ := newTestRegistry()
	_, err := reg.Execute(context.Background(), call("plan", "create_block",
		`{"started_at":"2026-07-20 10:00","ended_at":"2026-07-20 09:00","label":"x"}`))
	require.ErrorIs(t, err, ErrInvalidAction)
}

func TestRegistry_CatalogPrompt_ListsExpandedActions(t *testing.T) {
	reg, _, _ := newTestRegistry()
	cat := reg.CatalogPrompt()
	for _, a := range []string{
		"tasks.create_task", "tasks.delete_task", "tasks.complete_task", "tasks.create_project",
		"habits.create_habit", "habits.delete_habit",
		"finance.create_account", "finance.create_category", "finance.delete_transaction",
		"plan.create_block", "rutas.delete_mark",
	} {
		require.Contains(t, cat, a)
	}
}
