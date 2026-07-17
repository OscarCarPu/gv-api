package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gv-api/internal/assistant/llm"
	"gv-api/internal/finance"
	"gv-api/internal/finance/txtype"
	"gv-api/internal/habits"
	"gv-api/internal/plan"
	"gv-api/internal/rutas"
	"gv-api/internal/tasks"
	"gv-api/internal/varieties"

	"github.com/shopspring/decimal"
)

// Per-domain service interfaces: the registry depends only on the exact methods
// it uses (writes + the list reads it needs to resolve names to ids), keeping it
// unit-testable. The concrete domain *Service types satisfy these.
type (
	TaskService interface {
		CreateTask(ctx context.Context, req tasks.CreateTaskRequest) (tasks.TaskResponse, error)
		CreateProject(ctx context.Context, req tasks.CreateProjectRequest) (tasks.ProjectResponse, error)
		CreateTodo(ctx context.Context, req tasks.CreateTodoRequest) (tasks.TodoResponse, error)
		UpdateTask(ctx context.Context, req tasks.UpdateTaskRequest) (tasks.TaskResponse, error)
		UpdateProject(ctx context.Context, req tasks.UpdateProjectRequest) (tasks.ProjectResponse, error)
		DeleteTask(ctx context.Context, id int32) error
		DeleteProject(ctx context.Context, id int32) error
		ListOpenTasks(ctx context.Context) ([]tasks.TaskFastResponse, error)
		ListOpenProjects(ctx context.Context) ([]tasks.ProjectFastResponse, error)
	}
	HabitService interface {
		LogHabit(ctx context.Context, req habits.LogUpsertRequest) error
		CreateHabit(ctx context.Context, req habits.CreateHabitRequest) (habits.CreateHabitResponse, error)
		UpdateHabit(ctx context.Context, req habits.UpdateHabitRequest) (habits.CreateHabitResponse, error)
		DeleteHabit(ctx context.Context, id int32) error
		GetDailyView(ctx context.Context, dateStr string) ([]habits.HabitWithLog, error)
	}
	FinanceService interface {
		CreateTransaction(ctx context.Context, req finance.CreateTransactionRequest) (finance.Transaction, error)
		CreateAccount(ctx context.Context, req finance.CreateAccountRequest) (finance.Account, error)
		CreateCategory(ctx context.Context, req finance.CreateCategoryRequest) (finance.Category, error)
		UpdateTransaction(ctx context.Context, req finance.UpdateTransactionRequest) (finance.Transaction, error)
		UpdateAccount(ctx context.Context, req finance.UpdateAccountRequest) (finance.Account, error)
		UpdateCategory(ctx context.Context, req finance.UpdateCategoryRequest) (finance.Category, error)
		DeleteTransaction(ctx context.Context, id int32) error
		DeleteAccount(ctx context.Context, id int32) error
		DeleteCategory(ctx context.Context, id int32) error
		GetTransaction(ctx context.Context, id int32) (finance.Transaction, error)
		ListAccounts(ctx context.Context) ([]finance.Account, error)
		ListCategories(ctx context.Context) ([]finance.Category, error)
	}
	PlanService interface {
		Create(ctx context.Context, req plan.CreatePlanBlockRequest) (plan.PlanBlockResponse, error)
		Update(ctx context.Context, req plan.UpdatePlanBlockRequest) (plan.PlanBlockResponse, error)
		Delete(ctx context.Context, id int32) error
	}
	RutasService interface {
		Create(ctx context.Context, req rutas.CreateMarkRequest) (rutas.ConcelloMark, error)
		Update(ctx context.Context, req rutas.UpdateMarkRequest) (rutas.ConcelloMark, error)
		Delete(ctx context.Context, id int32) error
		List(ctx context.Context) ([]rutas.ConcelloMark, error)
	}
	VarietyService interface {
		Create(ctx context.Context, req varieties.CreateVarietyRequest) (varieties.Variety, error)
		Update(ctx context.Context, req varieties.UpdateVarietyRequest) (varieties.Variety, error)
		Delete(ctx context.Context, id int32) error
		List(ctx context.Context) ([]varieties.Variety, error)
	}
)

type actionSpec struct {
	catalog string // one-line description embedded in the LLM system prompt
	run     func(ctx context.Context, args json.RawMessage) (string, error)
}

// ActionRegistry validates and dispatches structured write actions to the
// existing domain services (reusing their validation/business logic) and
// resolves entity names to ids so the user can speak in natural language. It is
// the single source of truth for both the prompt catalog and the dispatch table.
type ActionRegistry struct {
	tasks     TaskService
	habits    HabitService
	finance   FinanceService
	plan      PlanService
	rutas     RutasService
	varieties VarietyService
	actions   map[string]actionSpec
}

// NewActionRegistry wires the actions to their services.
func NewActionRegistry(ts TaskService, hs HabitService, fs FinanceService, ps PlanService, rs RutasService, vs VarietyService) *ActionRegistry {
	r := &ActionRegistry{tasks: ts, habits: hs, finance: fs, plan: ps, rutas: rs, varieties: vs, actions: map[string]actionSpec{}}
	reg := func(key, catalog string, run func(context.Context, json.RawMessage) (string, error)) {
		r.actions[key] = actionSpec{catalog: catalog, run: run}
	}

	// Tasks
	reg("tasks.create_task",
		`tasks.create_task{ name:str, description?:str, project?:str(nombre), due_at?:date, priority?:1..5 } -> crea una tarea.`,
		r.createTask)
	reg("tasks.create_project",
		`tasks.create_project{ name:str, description?:str, due_at?:date } -> crea un proyecto.`,
		r.createProject)
	reg("tasks.create_todo",
		`tasks.create_todo{ task?:str(nombre), task_id?:int, name:str } -> añade un sub-todo a una tarea.`,
		r.createTodo)
	reg("tasks.complete_task",
		`tasks.complete_task{ task?:str(nombre), task_id?:int } -> marca una tarea como terminada (finished_at=ahora).`,
		r.completeTask)
	reg("tasks.update_task",
		`tasks.update_task{ task?:str(nombre), task_id?:int, new_name?:str, description?:str, due_at?:date, priority?:1..5 } -> edita una tarea.`,
		r.updateTask)
	reg("tasks.update_project",
		`tasks.update_project{ project?:str(nombre), project_id?:int, new_name?:str, description?:str, due_at?:date } -> edita un proyecto.`,
		r.updateProject)
	reg("tasks.delete_task",
		`tasks.delete_task{ task?:str(nombre), task_id?:int } -> borra una tarea.`,
		r.deleteTask)
	reg("tasks.delete_project",
		`tasks.delete_project{ project?:str(nombre), project_id?:int } -> borra un proyecto.`,
		r.deleteProject)

	// Habits
	reg("habits.log_habit",
		`habits.log_habit{ habit?:str(nombre), habit_id?:int, day?:date=hoy, value?:number=1 } -> registra un hábito un día.`,
		r.logHabit)
	reg("habits.create_habit",
		`habits.create_habit{ name:str, frequency?:daily|weekly|monthly, target_min?:number, target_max?:number } -> crea un hábito.`,
		r.createHabit)
	reg("habits.update_habit",
		`habits.update_habit{ habit?:str(nombre), habit_id?:int, new_name?:str, frequency?:daily|weekly|monthly, target_min?:number, target_max?:number } -> edita un hábito.`,
		r.updateHabit)
	reg("habits.delete_habit",
		`habits.delete_habit{ habit?:str(nombre), habit_id?:int } -> borra un hábito.`,
		r.deleteHabit)

	// Finance
	reg("finance.create_transaction",
		`finance.create_transaction{ type:income|expense|transfer, amount:decimal>0, account?:str(nombre), account_id?:int, category?:str(nombre), category_id?:int, to_account?:str(solo transfer), description?:str, occurred_at?:date } -> registra un movimiento. category.type debe == type.`,
		r.createTransaction)
	reg("finance.create_account",
		`finance.create_account{ name:str } -> crea una cuenta.`,
		r.createAccount)
	reg("finance.create_category",
		`finance.create_category{ name:str, type:income|expense|transfer, parent?:str(nombre) } -> crea una categoría.`,
		r.createCategory)
	reg("finance.update_transaction",
		`finance.update_transaction{ id:int, type?:income|expense|transfer, amount?:decimal>0, account?:str, category?:str, to_account?:str, description?:str, occurred_at?:date } -> edita un movimiento (requiere id).`,
		r.updateTransaction)
	reg("finance.update_account",
		`finance.update_account{ account?:str(nombre), account_id?:int, new_name:str } -> renombra una cuenta.`,
		r.updateAccount)
	reg("finance.update_category",
		`finance.update_category{ category?:str(nombre), category_id?:int, new_name?:str, type?:income|expense|transfer, parent?:str(nombre) } -> edita una categoría.`,
		r.updateCategory)
	reg("finance.delete_transaction",
		`finance.delete_transaction{ id:int } -> borra un movimiento (requiere id; búscalo antes con una consulta si hace falta).`,
		r.deleteTransaction)
	reg("finance.delete_account",
		`finance.delete_account{ account?:str(nombre), account_id?:int } -> borra una cuenta (debe estar sin movimientos).`,
		r.deleteAccount)
	reg("finance.delete_category",
		`finance.delete_category{ category?:str(nombre), category_id?:int } -> borra una categoría.`,
		r.deleteCategory)

	// Plan
	reg("plan.create_block",
		`plan.create_block{ started_at:datetime, ended_at:datetime, label:str, note?:str, task?:str(nombre) } -> crea un bloque de plan. datetime = "YYYY-MM-DD HH:MM".`,
		r.createPlanBlock)
	reg("plan.update_block",
		`plan.update_block{ id:int, started_at?:datetime, ended_at?:datetime, label?:str, note?:str } -> edita un bloque de plan.`,
		r.updatePlanBlock)
	reg("plan.delete_block",
		`plan.delete_block{ id:int } -> borra un bloque de plan.`,
		r.deletePlanBlock)

	// Rutas
	reg("rutas.create_mark",
		`rutas.create_mark{ name:str, visited_on:date, description?:str } -> marca un concello como visitado.`,
		r.createMark)
	reg("rutas.update_mark",
		`rutas.update_mark{ mark?:str(nombre), mark_id?:int, visited_on?:date, description?:str } -> edita una marca.`,
		r.updateMark)
	reg("rutas.delete_mark",
		`rutas.delete_mark{ mark?:str(nombre), mark_id?:int } -> borra una marca de concello.`,
		r.deleteMark)

	// Varieties (cata de variedades)
	reg("varieties.create_variety",
		`varieties.create_variety{ name:str, scent:0..10, flavor:0..10, power:0..10, quality:0..10, price:number, judge?:str, comments?:str } -> crea una variedad valorada.`,
		r.createVariety)
	reg("varieties.update_variety",
		`varieties.update_variety{ variety?:str(nombre), variety_id?:int, new_name?:str, scent?:0..10, flavor?:0..10, power?:0..10, quality?:0..10, price?:number, judge?:str, comments?:str } -> edita una variedad.`,
		r.updateVariety)
	reg("varieties.delete_variety",
		`varieties.delete_variety{ variety?:str(nombre), variety_id?:int } -> borra una variedad.`,
		r.deleteVariety)

	return r
}

// Execute dispatches an approved action and returns a Spanish confirmation.
func (r *ActionRegistry) Execute(ctx context.Context, call *llm.ActionCall) (string, error) {
	if call == nil {
		return "", fmt.Errorf("%w: missing action", ErrInvalidAction)
	}
	spec, ok := r.actions[call.Domain+"."+call.Operation]
	if !ok {
		return "", fmt.Errorf("%w: %s.%s", ErrUnsupportedAction, call.Domain, call.Operation)
	}
	return spec.run(ctx, call.Args)
}

// CatalogPrompt renders the ACTIONS section embedded in the LLM system prompt.
func (r *ActionRegistry) CatalogPrompt() string {
	keys := make([]string, 0, len(r.actions))
	for k := range r.actions {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("ACTIONS (para escrituras: emite exactamente una en action{domain,operation,args}; args deben cumplir el esquema)\n")
	b.WriteString("Referencia entidades por nombre cuando el usuario las nombre; el servidor resuelve el id.\n")
	for _, k := range keys {
		b.WriteString("  ")
		b.WriteString(r.actions[k].catalog)
		b.WriteString("\n")
	}
	return b.String()
}

// --- helpers ---

func decodeArgs(raw json.RawMessage, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidAction, err)
	}
	return nil
}

func parseArgDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, fmt.Errorf("%w: fecha inválida %q (usa YYYY-MM-DD)", ErrInvalidAction, s)
	}
	return &t, nil
}

func parseArgDateTime(s string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: fecha/hora inválida %q (usa \"YYYY-MM-DD HH:MM\")", ErrInvalidAction, s)
}

type namedRef struct {
	id   int32
	name string
}

// resolveRef finds an entity id by name: exact (case-insensitive) match first,
// then a unique substring match. Zero or multiple matches are errors so the user
// can disambiguate.
func resolveRef(refs []namedRef, name string) (int32, error) {
	q := strings.TrimSpace(strings.ToLower(name))
	if q == "" {
		return 0, fmt.Errorf("%w: falta el nombre o el id", ErrInvalidAction)
	}
	var exact, partial []namedRef
	for _, ref := range refs {
		n := strings.TrimSpace(strings.ToLower(ref.name))
		if n == q {
			exact = append(exact, ref)
		} else if strings.Contains(n, q) {
			partial = append(partial, ref)
		}
	}
	match := exact
	if len(match) == 0 {
		match = partial
	}
	switch len(match) {
	case 1:
		return match[0].id, nil
	case 0:
		return 0, fmt.Errorf("%w: no encontré %q", ErrInvalidAction, name)
	default:
		return 0, fmt.Errorf("%w: %q es ambiguo (%d coincidencias), sé más específico", ErrInvalidAction, name, len(match))
	}
}

// resolver builds refs from a list method and resolves id-or-name.
func resolveOrID[T any](ctx context.Context, idPtr *int32, name string, list func(context.Context) ([]T, error), id func(T) int32, nm func(T) string, entity string) (int32, error) {
	if idPtr != nil && *idPtr > 0 {
		return *idPtr, nil
	}
	if strings.TrimSpace(name) == "" {
		return 0, fmt.Errorf("%w: indica el %s por nombre o id", ErrInvalidAction, entity)
	}
	items, err := list(ctx)
	if err != nil {
		return 0, err
	}
	refs := make([]namedRef, len(items))
	for i, it := range items {
		refs[i] = namedRef{id: id(it), name: nm(it)}
	}
	return resolveRef(refs, name)
}

// resolveItem is like resolveOrID but returns the full matched item — needed for
// full-replace updates that must read current values before overriding.
func resolveItem[T any](ctx context.Context, idPtr *int32, name string, list func(context.Context) ([]T, error), id func(T) int32, nm func(T) string, entity string) (T, error) {
	var zero T
	items, err := list(ctx)
	if err != nil {
		return zero, err
	}
	target := int32(0)
	if idPtr != nil && *idPtr > 0 {
		target = *idPtr
	} else {
		refs := make([]namedRef, len(items))
		for i, it := range items {
			refs[i] = namedRef{id: id(it), name: nm(it)}
		}
		rid, rerr := resolveRef(refs, name)
		if rerr != nil {
			return zero, rerr
		}
		target = rid
	}
	for _, it := range items {
		if id(it) == target {
			return it, nil
		}
	}
	return zero, fmt.Errorf("%w: no encontré %s", ErrInvalidAction, entity)
}

func (r *ActionRegistry) resolveVariety(ctx context.Context, idPtr *int32, name string) (varieties.Variety, error) {
	return resolveItem(ctx, idPtr, name, r.varieties.List,
		func(v varieties.Variety) int32 { return v.ID }, func(v varieties.Variety) string { return v.Name }, "variedad")
}

func (r *ActionRegistry) resolveTask(ctx context.Context, idPtr *int32, name string) (int32, error) {
	return resolveOrID(ctx, idPtr, name, r.tasks.ListOpenTasks,
		func(t tasks.TaskFastResponse) int32 { return t.ID }, func(t tasks.TaskFastResponse) string { return t.Name }, "tarea")
}

func (r *ActionRegistry) resolveProject(ctx context.Context, idPtr *int32, name string) (int32, error) {
	return resolveOrID(ctx, idPtr, name, r.tasks.ListOpenProjects,
		func(p tasks.ProjectFastResponse) int32 { return p.ID }, func(p tasks.ProjectFastResponse) string { return p.Name }, "proyecto")
}

func (r *ActionRegistry) resolveHabit(ctx context.Context, idPtr *int32, name string) (int32, error) {
	return resolveOrID(ctx, idPtr, name, func(c context.Context) ([]habits.HabitWithLog, error) { return r.habits.GetDailyView(c, "") },
		func(h habits.HabitWithLog) int32 { return h.ID }, func(h habits.HabitWithLog) string { return h.Name }, "hábito")
}

func (r *ActionRegistry) resolveAccount(ctx context.Context, idPtr *int32, name string) (int32, error) {
	return resolveOrID(ctx, idPtr, name, r.finance.ListAccounts,
		func(a finance.Account) int32 { return a.ID }, func(a finance.Account) string { return a.Name }, "cuenta")
}

func (r *ActionRegistry) resolveCategory(ctx context.Context, idPtr *int32, name string) (int32, error) {
	return resolveOrID(ctx, idPtr, name, r.finance.ListCategories,
		func(c finance.Category) int32 { return c.ID }, func(c finance.Category) string { return c.Name }, "categoría")
}

func (r *ActionRegistry) resolveMark(ctx context.Context, idPtr *int32, name string) (int32, error) {
	return resolveOrID(ctx, idPtr, name, r.rutas.List,
		func(m rutas.ConcelloMark) int32 { return m.ID }, func(m rutas.ConcelloMark) string { return m.Name }, "marca")
}

func requireName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("%w: name es obligatorio", ErrInvalidAction)
	}
	if len(s) > 200 {
		return "", fmt.Errorf("%w: name demasiado largo", ErrInvalidAction)
	}
	return s, nil
}

func ptr[T any](v T) *T { return &v }

// --- Tasks handlers ---

func (r *ActionRegistry) createTask(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		Project     string  `json:"project"`
		ProjectID   *int32  `json:"project_id"`
		DueAt       string  `json:"due_at"`
		Priority    *int32  `json:"priority"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	name, err := requireName(a.Name)
	if err != nil {
		return "", err
	}
	if a.Priority != nil && (*a.Priority < 1 || *a.Priority > 5) {
		return "", fmt.Errorf("%w: priority debe estar entre 1 y 5", ErrInvalidAction)
	}
	due, err := parseArgDate(a.DueAt)
	if err != nil {
		return "", err
	}
	var projectID *int32
	if a.ProjectID != nil || strings.TrimSpace(a.Project) != "" {
		id, rerr := r.resolveProject(ctx, a.ProjectID, a.Project)
		if rerr != nil {
			return "", rerr
		}
		projectID = &id
	}
	resp, err := r.tasks.CreateTask(ctx, tasks.CreateTaskRequest{
		Name: name, Description: a.Description, ProjectID: projectID, DueAt: due, Priority: a.Priority,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Tarea creada: %q (id %d).", resp.Name, resp.ID), nil
}

func (r *ActionRegistry) createProject(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Name        string  `json:"name"`
		Description *string `json:"description"`
		DueAt       string  `json:"due_at"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	name, err := requireName(a.Name)
	if err != nil {
		return "", err
	}
	due, err := parseArgDate(a.DueAt)
	if err != nil {
		return "", err
	}
	resp, err := r.tasks.CreateProject(ctx, tasks.CreateProjectRequest{Name: name, Description: a.Description, DueAt: due})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Proyecto creado: %q (id %d).", resp.Name, resp.ID), nil
}

func (r *ActionRegistry) createTodo(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Task   string `json:"task"`
		TaskID *int32 `json:"task_id"`
		Name   string `json:"name"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	name, err := requireName(a.Name)
	if err != nil {
		return "", err
	}
	taskID, err := r.resolveTask(ctx, a.TaskID, a.Task)
	if err != nil {
		return "", err
	}
	resp, err := r.tasks.CreateTodo(ctx, tasks.CreateTodoRequest{TaskID: taskID, Name: name})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Sub-todo creado: %q (id %d).", resp.Name, resp.ID), nil
}

func (r *ActionRegistry) completeTask(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Task   string `json:"task"`
		TaskID *int32 `json:"task_id"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	id, err := r.resolveTask(ctx, a.TaskID, a.Task)
	if err != nil {
		return "", err
	}
	now := time.Now()
	if _, err := r.tasks.UpdateTask(ctx, tasks.UpdateTaskRequest{ID: id, FinishedAt: tasks.NullableTime{Value: &now, Set: true}}); err != nil {
		return "", err
	}
	return fmt.Sprintf("Tarea %d marcada como terminada.", id), nil
}

func (r *ActionRegistry) updateTask(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Task        string  `json:"task"`
		TaskID      *int32  `json:"task_id"`
		NewName     *string `json:"new_name"`
		Description *string `json:"description"`
		DueAt       string  `json:"due_at"`
		Priority    *int32  `json:"priority"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	id, err := r.resolveTask(ctx, a.TaskID, a.Task)
	if err != nil {
		return "", err
	}
	req := tasks.UpdateTaskRequest{ID: id, Name: a.NewName, Description: a.Description, Priority: a.Priority}
	if a.Priority != nil && (*a.Priority < 1 || *a.Priority > 5) {
		return "", fmt.Errorf("%w: priority debe estar entre 1 y 5", ErrInvalidAction)
	}
	if a.DueAt != "" {
		due, derr := parseArgDate(a.DueAt)
		if derr != nil {
			return "", derr
		}
		req.DueAt = tasks.NullableTime{Value: due, Set: true}
	}
	if _, err := r.tasks.UpdateTask(ctx, req); err != nil {
		return "", err
	}
	return fmt.Sprintf("Tarea %d actualizada.", id), nil
}

func (r *ActionRegistry) deleteTask(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Task   string `json:"task"`
		TaskID *int32 `json:"task_id"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	id, err := r.resolveTask(ctx, a.TaskID, a.Task)
	if err != nil {
		return "", err
	}
	if err := r.tasks.DeleteTask(ctx, id); err != nil {
		return "", err
	}
	return fmt.Sprintf("Tarea %d borrada.", id), nil
}

func (r *ActionRegistry) deleteProject(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Project   string `json:"project"`
		ProjectID *int32 `json:"project_id"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	id, err := r.resolveProject(ctx, a.ProjectID, a.Project)
	if err != nil {
		return "", err
	}
	if err := r.tasks.DeleteProject(ctx, id); err != nil {
		return "", err
	}
	return fmt.Sprintf("Proyecto %d borrado.", id), nil
}

// --- Habits handlers ---

func (r *ActionRegistry) logHabit(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Habit   string   `json:"habit"`
		HabitID *int32   `json:"habit_id"`
		Day     string   `json:"day"`
		Value   *float32 `json:"value"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	id, err := r.resolveHabit(ctx, a.HabitID, a.Habit)
	if err != nil {
		return "", err
	}
	day := a.Day
	if day == "" {
		day = time.Now().Format("2006-01-02")
	} else if _, err := time.Parse("2006-01-02", day); err != nil {
		return "", fmt.Errorf("%w: day inválido %q (usa YYYY-MM-DD)", ErrInvalidAction, day)
	}
	value := float32(1)
	if a.Value != nil {
		value = *a.Value
	}
	if err := r.habits.LogHabit(ctx, habits.LogUpsertRequest{HabitID: id, Date: day, Value: value}); err != nil {
		return "", err
	}
	return fmt.Sprintf("Hábito %d registrado el %s (valor %v).", id, day, value), nil
}

func (r *ActionRegistry) createHabit(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Name      string   `json:"name"`
		Frequency *string  `json:"frequency"`
		TargetMin *float32 `json:"target_min"`
		TargetMax *float32 `json:"target_max"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	name, err := requireName(a.Name)
	if err != nil {
		return "", err
	}
	resp, err := r.habits.CreateHabit(ctx, habits.CreateHabitRequest{
		Name: name, Frequency: a.Frequency, TargetMin: a.TargetMin, TargetMax: a.TargetMax,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Hábito creado: %q (id %d).", resp.Name, resp.ID), nil
}

func (r *ActionRegistry) deleteHabit(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Habit   string `json:"habit"`
		HabitID *int32 `json:"habit_id"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	id, err := r.resolveHabit(ctx, a.HabitID, a.Habit)
	if err != nil {
		return "", err
	}
	if err := r.habits.DeleteHabit(ctx, id); err != nil {
		return "", err
	}
	return fmt.Sprintf("Hábito %d borrado.", id), nil
}

// --- Finance handlers ---

func (r *ActionRegistry) createTransaction(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Type        string          `json:"type"`
		Amount      decimal.Decimal `json:"amount"`
		Account     string          `json:"account"`
		AccountID   *int32          `json:"account_id"`
		Category    string          `json:"category"`
		CategoryID  *int32          `json:"category_id"`
		ToAccount   string          `json:"to_account"`
		ToAccountID *int32          `json:"to_account_id"`
		Description *string         `json:"description"`
		OccurredAt  string          `json:"occurred_at"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	t := txtype.Type(a.Type)
	if !t.Valid() {
		return "", fmt.Errorf("%w: type debe ser income, expense o transfer", ErrInvalidAction)
	}
	if !a.Amount.IsPositive() {
		return "", fmt.Errorf("%w: amount debe ser mayor que 0", ErrInvalidAction)
	}
	accountID, err := r.resolveAccount(ctx, a.AccountID, a.Account)
	if err != nil {
		return "", err
	}
	categoryID, err := r.resolveCategory(ctx, a.CategoryID, a.Category)
	if err != nil {
		return "", err
	}
	var toAccountID *int32
	switch t {
	case txtype.Transfer:
		id, terr := r.resolveAccount(ctx, a.ToAccountID, a.ToAccount)
		if terr != nil {
			return "", terr
		}
		if id == accountID {
			return "", fmt.Errorf("%w: la cuenta destino debe diferir de la origen", ErrInvalidAction)
		}
		toAccountID = &id
	default:
		if a.ToAccountID != nil || strings.TrimSpace(a.ToAccount) != "" {
			return "", fmt.Errorf("%w: to_account solo aplica a transfer", ErrInvalidAction)
		}
	}
	occurred, err := parseArgDate(a.OccurredAt)
	if err != nil {
		return "", err
	}
	tx, err := r.finance.CreateTransaction(ctx, finance.CreateTransactionRequest{
		Type: t, Amount: a.Amount, AccountID: accountID, ToAccountID: toAccountID,
		CategoryID: &categoryID, Description: a.Description, OccurredAt: occurred,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Movimiento creado: %s de %s (id %d).", tx.Type, tx.Amount.String(), tx.ID), nil
}

func (r *ActionRegistry) createAccount(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Name string `json:"name"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	name := strings.TrimSpace(a.Name)
	if name == "" || len(name) > 40 {
		return "", fmt.Errorf("%w: name obligatorio (máx 40)", ErrInvalidAction)
	}
	acc, err := r.finance.CreateAccount(ctx, finance.CreateAccountRequest{Name: name})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Cuenta creada: %q (id %d).", acc.Name, acc.ID), nil
}

func (r *ActionRegistry) createCategory(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		Parent string `json:"parent"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	name := strings.TrimSpace(a.Name)
	if name == "" || len(name) > 40 {
		return "", fmt.Errorf("%w: name obligatorio (máx 40)", ErrInvalidAction)
	}
	t := txtype.Type(a.Type)
	if !t.Valid() {
		return "", fmt.Errorf("%w: type debe ser income, expense o transfer", ErrInvalidAction)
	}
	var parentID *int32
	if strings.TrimSpace(a.Parent) != "" {
		id, perr := r.resolveCategory(ctx, nil, a.Parent)
		if perr != nil {
			return "", perr
		}
		parentID = &id
	}
	cat, err := r.finance.CreateCategory(ctx, finance.CreateCategoryRequest{Name: name, Type: t, ParentID: parentID})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Categoría creada: %q (id %d).", cat.Name, cat.ID), nil
}

func (r *ActionRegistry) deleteTransaction(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		ID int32 `json:"id"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	if a.ID <= 0 {
		return "", fmt.Errorf("%w: id es obligatorio", ErrInvalidAction)
	}
	if err := r.finance.DeleteTransaction(ctx, a.ID); err != nil {
		return "", err
	}
	return fmt.Sprintf("Movimiento %d borrado.", a.ID), nil
}

func (r *ActionRegistry) deleteAccount(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Account   string `json:"account"`
		AccountID *int32 `json:"account_id"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	id, err := r.resolveAccount(ctx, a.AccountID, a.Account)
	if err != nil {
		return "", err
	}
	if err := r.finance.DeleteAccount(ctx, id); err != nil {
		return "", err
	}
	return fmt.Sprintf("Cuenta %d borrada.", id), nil
}

func (r *ActionRegistry) deleteCategory(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Category   string `json:"category"`
		CategoryID *int32 `json:"category_id"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	id, err := r.resolveCategory(ctx, a.CategoryID, a.Category)
	if err != nil {
		return "", err
	}
	if err := r.finance.DeleteCategory(ctx, id); err != nil {
		return "", err
	}
	return fmt.Sprintf("Categoría %d borrada.", id), nil
}

// --- Plan handlers ---

func (r *ActionRegistry) createPlanBlock(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		StartedAt string `json:"started_at"`
		EndedAt   string `json:"ended_at"`
		Label     string `json:"label"`
		Note      string `json:"note"`
		Task      string `json:"task"`
		TaskID    *int32 `json:"task_id"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	label := strings.TrimSpace(a.Label)
	if label == "" || len(label) > 200 {
		return "", fmt.Errorf("%w: label obligatorio (máx 200)", ErrInvalidAction)
	}
	start, err := parseArgDateTime(a.StartedAt)
	if err != nil {
		return "", err
	}
	end, err := parseArgDateTime(a.EndedAt)
	if err != nil {
		return "", err
	}
	if !end.After(start) {
		return "", fmt.Errorf("%w: ended_at debe ser posterior a started_at", ErrInvalidAction)
	}
	var taskID *int32
	if a.TaskID != nil || strings.TrimSpace(a.Task) != "" {
		id, terr := r.resolveTask(ctx, a.TaskID, a.Task)
		if terr != nil {
			return "", terr
		}
		taskID = &id
	}
	req := plan.CreatePlanBlockRequest{StartedAt: start, EndedAt: end, Label: &label, TaskID: taskID}
	if strings.TrimSpace(a.Note) != "" {
		req.Note = ptr(a.Note)
	}
	block, err := r.plan.Create(ctx, req)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Bloque de plan creado: %q (id %d).", block.Label, block.ID), nil
}

func (r *ActionRegistry) deletePlanBlock(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		ID int32 `json:"id"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	if a.ID <= 0 {
		return "", fmt.Errorf("%w: id es obligatorio", ErrInvalidAction)
	}
	if err := r.plan.Delete(ctx, a.ID); err != nil {
		return "", err
	}
	return fmt.Sprintf("Bloque de plan %d borrado.", a.ID), nil
}

// --- Rutas handlers ---

func (r *ActionRegistry) createMark(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Name        string `json:"name"`
		VisitedOn   string `json:"visited_on"`
		Description string `json:"description"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	name, err := requireName(a.Name)
	if err != nil {
		return "", err
	}
	if _, terr := time.Parse("2006-01-02", a.VisitedOn); terr != nil {
		return "", fmt.Errorf("%w: visited_on inválido (usa YYYY-MM-DD)", ErrInvalidAction)
	}
	mark, err := r.rutas.Create(ctx, rutas.CreateMarkRequest{Name: name, VisitedOn: a.VisitedOn, Description: a.Description})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Concello marcado: %q el %s (id %d).", mark.Name, a.VisitedOn, mark.ID), nil
}

func (r *ActionRegistry) deleteMark(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Mark   string `json:"mark"`
		MarkID *int32 `json:"mark_id"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	id, err := r.resolveMark(ctx, a.MarkID, a.Mark)
	if err != nil {
		return "", err
	}
	if err := r.rutas.Delete(ctx, id); err != nil {
		return "", err
	}
	return fmt.Sprintf("Marca %d borrada.", id), nil
}

func (r *ActionRegistry) updateMark(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Mark        string `json:"mark"`
		MarkID      *int32 `json:"mark_id"`
		VisitedOn   string `json:"visited_on"`
		Description string `json:"description"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	cur, err := resolveItem(ctx, a.MarkID, a.Mark, r.rutas.List,
		func(m rutas.ConcelloMark) int32 { return m.ID }, func(m rutas.ConcelloMark) string { return m.Name }, "marca")
	if err != nil {
		return "", err
	}
	req := rutas.UpdateMarkRequest{ID: cur.ID, VisitedOn: cur.VisitedOn.Format("2006-01-02"), Description: cur.Description}
	if a.VisitedOn != "" {
		if _, terr := time.Parse("2006-01-02", a.VisitedOn); terr != nil {
			return "", fmt.Errorf("%w: visited_on inválido (usa YYYY-MM-DD)", ErrInvalidAction)
		}
		req.VisitedOn = a.VisitedOn
	}
	if strings.TrimSpace(a.Description) != "" {
		req.Description = a.Description
	}
	if _, err := r.rutas.Update(ctx, req); err != nil {
		return "", err
	}
	return fmt.Sprintf("Marca %q actualizada.", cur.Name), nil
}

// --- Tasks / Habits / Finance / Plan updates ---

func (r *ActionRegistry) updateProject(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Project     string  `json:"project"`
		ProjectID   *int32  `json:"project_id"`
		NewName     *string `json:"new_name"`
		Description *string `json:"description"`
		DueAt       string  `json:"due_at"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	id, err := r.resolveProject(ctx, a.ProjectID, a.Project)
	if err != nil {
		return "", err
	}
	req := tasks.UpdateProjectRequest{ID: id, Name: a.NewName, Description: a.Description}
	if a.DueAt != "" {
		due, derr := parseArgDate(a.DueAt)
		if derr != nil {
			return "", derr
		}
		req.DueAt = tasks.NullableTime{Value: due, Set: true}
	}
	if _, err := r.tasks.UpdateProject(ctx, req); err != nil {
		return "", err
	}
	return fmt.Sprintf("Proyecto %d actualizado.", id), nil
}

func (r *ActionRegistry) updateHabit(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Habit     string   `json:"habit"`
		HabitID   *int32   `json:"habit_id"`
		NewName   *string  `json:"new_name"`
		Frequency *string  `json:"frequency"`
		TargetMin *float32 `json:"target_min"`
		TargetMax *float32 `json:"target_max"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	cur, err := resolveItem(ctx, a.HabitID, a.Habit, func(c context.Context) ([]habits.HabitWithLog, error) { return r.habits.GetDailyView(c, "") },
		func(h habits.HabitWithLog) int32 { return h.ID }, func(h habits.HabitWithLog) string { return h.Name }, "hábito")
	if err != nil {
		return "", err
	}
	// Start from current values (full-replace update), then override.
	req := habits.UpdateHabitRequest{
		ID: cur.ID, Name: cur.Name, Description: cur.Description, Frequency: cur.Frequency,
		TargetMin: cur.TargetMin, TargetMax: cur.TargetMax, RecordingRequired: cur.RecordingRequired,
	}
	if a.NewName != nil {
		req.Name = strings.TrimSpace(*a.NewName)
	}
	if a.Frequency != nil {
		req.Frequency = *a.Frequency
	}
	if a.TargetMin != nil {
		req.TargetMin = a.TargetMin
	}
	if a.TargetMax != nil {
		req.TargetMax = a.TargetMax
	}
	if _, err := r.habits.UpdateHabit(ctx, req); err != nil {
		return "", err
	}
	return fmt.Sprintf("Hábito %q actualizado.", cur.Name), nil
}

func (r *ActionRegistry) updateTransaction(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		ID          int32            `json:"id"`
		Type        *string          `json:"type"`
		Amount      *decimal.Decimal `json:"amount"`
		Account     string           `json:"account"`
		AccountID   *int32           `json:"account_id"`
		Category    string           `json:"category"`
		CategoryID  *int32           `json:"category_id"`
		Description *string          `json:"description"`
		OccurredAt  string           `json:"occurred_at"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	if a.ID <= 0 {
		return "", fmt.Errorf("%w: id es obligatorio", ErrInvalidAction)
	}
	cur, err := r.finance.GetTransaction(ctx, a.ID)
	if err != nil {
		return "", err
	}
	req := finance.UpdateTransactionRequest{
		ID: cur.ID, Type: cur.Type, Amount: cur.Amount, AccountID: cur.AccountID,
		ToAccountID: cur.ToAccountID, CategoryID: cur.CategoryID, Description: cur.Description, OccurredAt: cur.OccurredAt,
	}
	if a.Type != nil {
		t := txtype.Type(*a.Type)
		if !t.Valid() {
			return "", fmt.Errorf("%w: type debe ser income, expense o transfer", ErrInvalidAction)
		}
		req.Type = t
	}
	if a.Amount != nil {
		if !a.Amount.IsPositive() {
			return "", fmt.Errorf("%w: amount debe ser mayor que 0", ErrInvalidAction)
		}
		req.Amount = *a.Amount
	}
	if a.AccountID != nil || strings.TrimSpace(a.Account) != "" {
		id, rerr := r.resolveAccount(ctx, a.AccountID, a.Account)
		if rerr != nil {
			return "", rerr
		}
		req.AccountID = id
	}
	if a.CategoryID != nil || strings.TrimSpace(a.Category) != "" {
		id, rerr := r.resolveCategory(ctx, a.CategoryID, a.Category)
		if rerr != nil {
			return "", rerr
		}
		req.CategoryID = &id
	}
	if a.Description != nil {
		req.Description = a.Description
	}
	if a.OccurredAt != "" {
		occ, derr := parseArgDate(a.OccurredAt)
		if derr != nil {
			return "", derr
		}
		req.OccurredAt = *occ
	}
	if _, err := r.finance.UpdateTransaction(ctx, req); err != nil {
		return "", err
	}
	return fmt.Sprintf("Movimiento %d actualizado.", a.ID), nil
}

func (r *ActionRegistry) updateAccount(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Account   string `json:"account"`
		AccountID *int32 `json:"account_id"`
		NewName   string `json:"new_name"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	name := strings.TrimSpace(a.NewName)
	if name == "" || len(name) > 40 {
		return "", fmt.Errorf("%w: new_name obligatorio (máx 40)", ErrInvalidAction)
	}
	id, err := r.resolveAccount(ctx, a.AccountID, a.Account)
	if err != nil {
		return "", err
	}
	if _, err := r.finance.UpdateAccount(ctx, finance.UpdateAccountRequest{ID: id, Name: name}); err != nil {
		return "", err
	}
	return fmt.Sprintf("Cuenta %d renombrada a %q.", id, name), nil
}

func (r *ActionRegistry) updateCategory(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Category   string  `json:"category"`
		CategoryID *int32  `json:"category_id"`
		NewName    *string `json:"new_name"`
		Type       *string `json:"type"`
		Parent     string  `json:"parent"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	cur, err := resolveItem(ctx, a.CategoryID, a.Category, r.finance.ListCategories,
		func(c finance.Category) int32 { return c.ID }, func(c finance.Category) string { return c.Name }, "categoría")
	if err != nil {
		return "", err
	}
	req := finance.UpdateCategoryRequest{ID: cur.ID, Name: cur.Name, ParentID: cur.ParentID, Type: cur.Type}
	if a.NewName != nil {
		req.Name = strings.TrimSpace(*a.NewName)
	}
	if a.Type != nil {
		t := txtype.Type(*a.Type)
		if !t.Valid() {
			return "", fmt.Errorf("%w: type debe ser income, expense o transfer", ErrInvalidAction)
		}
		req.Type = t
	}
	if strings.TrimSpace(a.Parent) != "" {
		pid, perr := r.resolveCategory(ctx, nil, a.Parent)
		if perr != nil {
			return "", perr
		}
		req.ParentID = &pid
	}
	if _, err := r.finance.UpdateCategory(ctx, req); err != nil {
		return "", err
	}
	return fmt.Sprintf("Categoría %d actualizada.", cur.ID), nil
}

func (r *ActionRegistry) updatePlanBlock(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		ID        int32   `json:"id"`
		StartedAt string  `json:"started_at"`
		EndedAt   string  `json:"ended_at"`
		Label     *string `json:"label"`
		Note      *string `json:"note"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	if a.ID <= 0 {
		return "", fmt.Errorf("%w: id es obligatorio", ErrInvalidAction)
	}
	req := plan.UpdatePlanBlockRequest{ID: a.ID, Label: a.Label, Note: a.Note}
	if a.StartedAt != "" {
		t, terr := parseArgDateTime(a.StartedAt)
		if terr != nil {
			return "", terr
		}
		req.StartedAt = &t
	}
	if a.EndedAt != "" {
		t, terr := parseArgDateTime(a.EndedAt)
		if terr != nil {
			return "", terr
		}
		req.EndedAt = &t
	}
	if _, err := r.plan.Update(ctx, req); err != nil {
		return "", err
	}
	return fmt.Sprintf("Bloque de plan %d actualizado.", a.ID), nil
}

// --- Varieties handlers ---

func (r *ActionRegistry) createVariety(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Name     string  `json:"name"`
		Scent    float32 `json:"scent"`
		Flavor   float32 `json:"flavor"`
		Power    float32 `json:"power"`
		Quality  float32 `json:"quality"`
		Price    float32 `json:"price"`
		Judge    string  `json:"judge"`
		Comments *string `json:"comments"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	name, err := requireName(a.Name)
	if err != nil {
		return "", err
	}
	judge := strings.TrimSpace(a.Judge)
	if judge == "" {
		judge = "Asistente"
	}
	v, err := r.varieties.Create(ctx, varieties.CreateVarietyRequest{
		Name: name, Scent: a.Scent, Flavor: a.Flavor, Power: a.Power, Quality: a.Quality,
		Price: a.Price, Judge: judge, Comments: a.Comments,
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Variedad creada: %q (id %d, score %.2f).", v.Name, v.ID, v.Score), nil
}

func (r *ActionRegistry) updateVariety(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Variety   string   `json:"variety"`
		VarietyID *int32   `json:"variety_id"`
		NewName   *string  `json:"new_name"`
		Scent     *float32 `json:"scent"`
		Flavor    *float32 `json:"flavor"`
		Power     *float32 `json:"power"`
		Quality   *float32 `json:"quality"`
		Price     *float32 `json:"price"`
		Judge     *string  `json:"judge"`
		Comments  *string  `json:"comments"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	cur, err := r.resolveVariety(ctx, a.VarietyID, a.Variety)
	if err != nil {
		return "", err
	}
	req := varieties.UpdateVarietyRequest{
		ID: cur.ID, Name: cur.Name, Scent: cur.Scent, Flavor: cur.Flavor, Power: cur.Power,
		Quality: cur.Quality, Price: cur.Price, Judge: cur.Judge, Comments: cur.Comments,
	}
	if a.NewName != nil {
		req.Name = strings.TrimSpace(*a.NewName)
	}
	if a.Scent != nil {
		req.Scent = *a.Scent
	}
	if a.Flavor != nil {
		req.Flavor = *a.Flavor
	}
	if a.Power != nil {
		req.Power = *a.Power
	}
	if a.Quality != nil {
		req.Quality = *a.Quality
	}
	if a.Price != nil {
		req.Price = *a.Price
	}
	if a.Judge != nil {
		req.Judge = *a.Judge
	}
	if a.Comments != nil {
		req.Comments = a.Comments
	}
	v, err := r.varieties.Update(ctx, req)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Variedad %q actualizada (score %.2f).", v.Name, v.Score), nil
}

func (r *ActionRegistry) deleteVariety(ctx context.Context, raw json.RawMessage) (string, error) {
	var a struct {
		Variety   string `json:"variety"`
		VarietyID *int32 `json:"variety_id"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		return "", err
	}
	cur, err := r.resolveVariety(ctx, a.VarietyID, a.Variety)
	if err != nil {
		return "", err
	}
	if err := r.varieties.Delete(ctx, cur.ID); err != nil {
		return "", err
	}
	return fmt.Sprintf("Variedad %q borrada.", cur.Name), nil
}
