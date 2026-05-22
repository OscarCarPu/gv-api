# gv-api — Issues & Proposed Fixes

---

## Bugs

### 1. `GetProjectChildren` and `GetTaskTimeEntries` return 404 on empty results

**Files:**
- `internal/tasks/repository.go:510-512` — `GetProjectChildren`
- `internal/tasks/repository.go:651-653` — `GetTaskTimeEntries`

**Problem:**  
Both methods return `ErrNotFound` when the query returns zero rows. But zero rows is a valid state — a project can exist with no children, and a task can exist with no time entries. The handler maps `ErrNotFound` to HTTP 404, so a client asking "what are the children of project 5?" gets a 404 even though project 5 exists. This is indistinguishable from "project 5 does not exist."

```go
// repository.go:510 — current
if len(descendants) == 0 {
    return ProjectChildrenResponse{}, ErrNotFound  // wrong: project exists, it just has no children
}

// repository.go:651 — current
if len(rows) == 0 {
    return TaskTimeEntriesResponse{}, ErrNotFound  // wrong: task exists, it just has no entries
}
```

**Fix:**  
`GetProjectChildren` should verify the project exists (first row of the CTE is always the root project itself — if `descendants` is empty, the project truly doesn't exist). `GetTaskTimeEntries` needs a separate existence check since `GetTimeEntriesByTaskID` is a LEFT JOIN that returns a row with null time entry fields when the task exists but has no entries.

```go
// GetProjectChildren — if descendants is empty the project itself wasn't found
if len(descendants) == 0 {
    return ProjectChildrenResponse{}, ErrNotFound  // here it IS correct: project doesn't exist
}
// no further ErrNotFound — an empty children list is valid

// GetTaskTimeEntries — the LEFT JOIN already handles it:
// if rows is empty, the task doesn't exist; if first row has nil TimeEntryID, task exists but no entries
if len(rows) == 0 {
    return TaskTimeEntriesResponse{}, ErrNotFound  // task doesn't exist
}
// then in the loop, skip nil TimeEntryID rows → entries stays []TimeEntryResponse{}
// this already happens at line 659, so just remove the early ErrNotFound on len==0
```

For `GetTaskTimeEntries` the fix is simply to remove the `len(rows) == 0` early return — the existing `row.TimeEntryID == nil` check in the loop already handles the no-entries case and `entries` will correctly be `[]TimeEntryResponse{}`.

---

### 2. Delete handlers silently return 204 on non-existent IDs

**File:** `internal/tasks/handler.go:469-519`

**Problem:**  
`DeleteProject`, `DeleteTask`, `DeleteTodo`, and `DeleteTimeEntry` pass any error directly to `InternalError`, never checking for `ErrNotFound`. Deleting an ID that doesn't exist returns 204 No Content. This contradicts how `UpdateProject` and `UpdateTask` behave (both return 404 for missing IDs) and makes it impossible for a client to distinguish "deleted successfully" from "nothing was there."

```go
// handler.go — current (same pattern in all four delete handlers)
if err := h.service.DeleteProject(r.Context(), id); err != nil {
    response.InternalError(w, r, err, "Failed to delete project")  // ErrNotFound treated as 500
    return
}
```

**Fix:**  
Add the same `ErrNotFound` check the update handlers use. The repository's `DeleteX` methods currently don't return `ErrNotFound` because `sqlc`-generated delete queries don't check rows affected. Two options:

- **Option A (preferred):** Make the repository `DeleteX` methods use `pgx`'s `CommandTag.RowsAffected()` to detect no-op deletes and return `ErrNotFound`.
- **Option B:** Use `GetX` before `DeleteX` in the service layer (costs an extra query but no repository change).

With either option, the handler becomes:

```go
if err := h.service.DeleteProject(r.Context(), id); err != nil {
    if errors.Is(err, ErrNotFound) {
        response.Error(w, http.StatusNotFound, "project not found")
        return
    }
    response.InternalError(w, r, err, "Failed to delete project")
    return
}
```

---

### 3. Tasks with a non-active project ID are silently dropped from `GetActiveTree`

**File:** `internal/tasks/service.go:165-170`

**Problem:**  
When a task has a `project_id` that is not in the active projects set (e.g., its project was finished), it hits the `continue` branch and is neither attached to a project node nor added to orphans. The task simply disappears from the response with no signal to the client. A user's unfinished task becomes invisible.

```go
// service.go:165
if t.ProjectID != nil {
    if _, ok := projectNodes[*t.ProjectID]; ok {
        projectTasks[*t.ProjectID] = append(projectTasks[*t.ProjectID], node)
    }
    continue  // ← task with a finished/missing project is dropped entirely
}
orphanTasks = append(orphanTasks, node)
```

**Fix:**  
When a task has a `project_id` but the project is not in `projectNodes`, treat the task as an orphan instead of discarding it:

```go
if t.ProjectID != nil {
    if _, ok := projectNodes[*t.ProjectID]; ok {
        projectTasks[*t.ProjectID] = append(projectTasks[*t.ProjectID], node)
        continue
    }
    // project_id set but project not active — surface as orphan
}
orphanTasks = append(orphanTasks, node)
```

---

### 4. `unmarshalDepRefs` swallows JSON decode errors silently

**File:** `internal/tasks/repository.go` (search for `unmarshalDepRefs`)

**Problem:**  
When the stored JSON in the `depends_on` / `blocks` column cannot be decoded, the function logs a warning and returns an empty slice. The caller receives a response with no dependency data — no error, no HTTP 500, just missing fields. This masks data corruption in the database silently.

```go
// current
func unmarshalDepRefs(data []byte) []TaskDepRef {
    var refs []TaskDepRef
    if err := json.Unmarshal(data, &refs); err != nil {
        slog.Error("failed to unmarshal dep refs", "error", err)
        return nil  // caller gets empty deps, no error propagated
    }
    return refs
}
```

**Fix:**  
Change the signature to return an error and propagate it up through all callers (`GetTask`, `GetProjectChildren`, `GetTaskTimeEntries`, `GetTasksByDueDate`, `GetUnfinishedTasks`).

```go
func unmarshalDepRefs(data []byte) ([]TaskDepRef, error) {
    var refs []TaskDepRef
    if len(data) == 0 {
        return refs, nil
    }
    if err := json.Unmarshal(data, &refs); err != nil {
        return nil, fmt.Errorf("unmarshal dep refs: %w", err)
    }
    return refs, nil
}
```

---

## Security

### 5. Wildcard CORS

**File:** `cmd/api/main.go:100`

**Problem:**  
`AllowedOrigins: []string{"*"}` allows any browser origin to call every endpoint. Since this is not a public API, any website on the internet can make cross-origin requests with valid auth headers on behalf of a logged-in user.

```go
// main.go:99
r.Use(cors.Handler(cors.Options{
    AllowedOrigins: []string{"*"},  // too broad
    ...
}))
```

**Fix:**  
Add `AllowedOrigins` to config and validate it at startup:

```go
// config.go
AllowedOrigins []string  // loaded from ALLOWED_ORIGINS env var, comma-separated

// config validation
if len(cfg.AllowedOrigins) == 0 {
    return nil, errors.New("ALLOWED_ORIGINS must be set")
}

// main.go
AllowedOrigins: cfg.AllowedOrigins,
```

---

## Missing Tests

### 6. `internal/finance/handler_test.go` does not exist

**Problem:**  
`internal/finance/handler.go` is ~620 lines and contains non-trivial validation logic that is entirely untested at the HTTP layer:

- `validateTransaction`: transfer transactions require a `to_account_id`; amount must be positive; `type` must be one of a fixed enum.
- `CreateTransaction`: `occurred_at` is required but the validation only exists in `UpdateTransaction`.
- `ListTransactions`: parses 7 query parameters (`account_id`, `category_id`, `type`, `start_date`, `end_date`, `limit`, `offset`), each with their own validation.
- `GetEstimation`: `start_date` must be before `end_date`; `mode` must be one of two values.
- `GetNetWorthStats` / `GetMonthlyStats`: `granularity` enum validation.

**Fix:**  
Create `internal/finance/handler_test.go` following the same pattern as `internal/tasks/handler_test.go`. At minimum cover:

```
TestHandler_CreateTransaction_MissingOccurredAt        → 400
TestHandler_CreateTransaction_InvalidType              → 400
TestHandler_CreateTransaction_TransferMissingToAccount → 400
TestHandler_UpdateTransaction_TransferMissingToAccount → 400
TestHandler_ListTransactions_InvalidAccountID          → 400
TestHandler_ListTransactions_InvalidDateRange          → 400
TestHandler_GetEstimation_StartAfterEnd                → 400
TestHandler_GetEstimation_InvalidMode                  → 400
TestHandler_CreateCategory_SelfParent                  → 400
```

---

### 7. Delete handlers have no 404 test coverage

**File:** `internal/tasks/handler_test.go`

**Problem:**  
After fixing bug #2, the delete handlers will map `ErrNotFound` → 404. Currently there are no tests for this path on any of the four delete handlers (`DeleteProject`, `DeleteTask`, `DeleteTodo`, `DeleteTimeEntry`). Without tests, a future regression (accidentally removing the `ErrNotFound` check) would go undetected.

**Fix:**  
Add one `not found` sub-test per delete handler:

```go
t.Run("returns 404 when not found", func(t *testing.T) {
    svc := mocks.NewMockServiceInterface(t)
    svc.EXPECT().DeleteProject(mock.Anything, int32(1)).Return(tasks.ErrNotFound)

    req := httptest.NewRequest(http.MethodDelete, "/tasks/projects/1", nil)
    req = withURLParam(req, "project", "1")
    w := httptest.NewRecorder()

    h := tasks.NewHandler(svc)
    h.DeleteProject(w, req)

    assert.Equal(t, http.StatusNotFound, w.Code)
})
```

---

### 8. `GetTimeEntrySummary` service logic is untested

**File:** `internal/tasks/service.go` — `GetTimeEntrySummary`

**Problem:**  
The method computes `todayStart` and `weekStart` in the configured timezone, calls the repository, then calculates `WeeklyTargetSeconds` using `CalcDailyTargetSeconds` and attaches `CalcPace`. None of this is tested. A wrong timezone anchor or an off-by-one in the week calculation would be invisible.

**Fix:**  
Add `TestService_GetTimeEntrySummary` that:
1. Constructs a service with a known `*time.Location`.
2. Asserts the repo is called with `todayStart` and `weekStart` matching that location.
3. Verifies `WeeklyTargetSeconds` is `DailyTargetSeconds * 7` in the returned response.

```go
func TestService_GetTimeEntrySummary(t *testing.T) {
    loc, _ := time.LoadLocation("Europe/Madrid")
    now := time.Now().In(loc)
    expectedToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
    weekday := int(now.Weekday())
    if weekday == 0 { weekday = 7 }
    expectedWeek := expectedToday.AddDate(0, 0, -(weekday - 1))

    repo := mocks.NewMockRepository(t)
    repo.EXPECT().
        GetTimeEntrySummary(mock.Anything,
            mock.MatchedBy(func(t time.Time) bool { return t.Equal(expectedToday) }),
            mock.MatchedBy(func(t time.Time) bool { return t.Equal(expectedWeek) }),
        ).
        Return(tasks.TimeEntrySummaryResponse{DailyTargetSeconds: 3600}, nil)

    svc := tasks.NewService(repo, loc)
    got, err := svc.GetTimeEntrySummary(context.Background())
    require.NoError(t, err)
    assert.Equal(t, int32(3600*7), got.WeeklyTargetSeconds)
}
```

---

### 9. `UpdateTask` with `Blocks` replacement has no service test

**File:** `internal/tasks/service.go:95-98`

**Problem:**  
`UpdateTask` calls `ReplaceTaskBlocks` when `req.Blocks != nil`, but only the `DependsOn` path is exercised in `service_test.go`. If `ReplaceTaskBlocks` is accidentally broken or the condition is changed, no test catches it.

**Fix:**  
Add a test mirroring the existing `DependsOn` tests:

```go
func TestService_UpdateTask_ReplaceBlocks(t *testing.T) {
    repo := mocks.NewMockRepository(t)
    blocks := []int32{5, 6}
    repo.EXPECT().UpdateTask(mock.Anything, mock.Anything).Return(tasks.TaskResponse{ID: 1}, nil)
    repo.EXPECT().ReplaceTaskBlocks(mock.Anything, int32(1), blocks).Return(nil)
    repo.EXPECT().GetTaskDependencies(mock.Anything, int32(1)).
        Return([]tasks.TaskDepRef{}, []tasks.TaskDepRef{{ID: 5}, {ID: 6}}, false, nil)

    svc := tasks.NewService(repo, nil)
    got, err := svc.UpdateTask(context.Background(), tasks.UpdateTaskRequest{
        ID:     1,
        Blocks: &blocks,
    })
    require.NoError(t, err)
    require.Len(t, got.Blocks, 2)
}
```

---

### 10. `GetTimeEntryHistory` default-dates test asserts too weakly

**File:** `internal/tasks/service_test.go:491-508`

**Problem:**  
`TestService_GetTimeEntryHistory_DefaultDatesDaily` uses `mock.AnythingOfType("time.Time")` for both date arguments and then only asserts `assert.NotEmpty(t, resp.StartAt)`. The test passes even if the default window is wrong (e.g., 1 day instead of 30 days, or wrong timezone anchor). The date arithmetic is the only interesting thing this test covers, but it doesn't actually verify it.

```go
// current — does not verify the 30-day window
assert.NotEmpty(t, resp.StartAt)
assert.NotEmpty(t, resp.EndAt)
```

**Fix:**  
Use `mock.MatchedBy` to assert the actual time boundaries, and assert `StartAt` / `EndAt` in the response match the expected window:

```go
now := time.Now().UTC()
expectedEnd := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
expectedStart := expectedEnd.AddDate(0, 0, -30)

repo.EXPECT().
    GetTimeEntryHistory(mock.Anything, "day", "UTC",
        mock.MatchedBy(func(t time.Time) bool {
            return t.Equal(expectedStart)
        }),
        mock.MatchedBy(func(t time.Time) bool {
            return t.Equal(expectedEnd)
        }),
    ).
    Return([]history.Point{{Date: "2026-04-22", Value: 2.5}}, nil)

// ...
assert.Equal(t, expectedStart.Format("2006-01-02"), resp.StartAt)
assert.Equal(t, expectedEnd.Format("2006-01-02"), resp.EndAt)
```
