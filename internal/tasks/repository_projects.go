package tasks

import (
	"context"
	"errors"
	"sort"
	"time"

	"gv-api/internal/database/gvdb"
	"gv-api/internal/database/pgconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *PostgresRepository) CreateProject(ctx context.Context, name string, description *string, dueAt *time.Time, parentID *int32) (ProjectResponse, error) {
	var pgDueAt pgtype.Date
	if dueAt != nil {
		pgDueAt = pgtype.Date{Time: *dueAt, Valid: true}
	}

	row, err := r.q.CreateProject(ctx, gvdb.CreateProjectParams{
		Name:        name,
		Description: description,
		DueAt:       pgDueAt,
		ParentID:    parentID,
	})
	if err != nil {
		return ProjectResponse{}, err
	}

	return ProjectResponse{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       pgconv.DatePtr(row.DueAt),
		ParentID:    row.ParentID,
	}, nil
}

func (r *PostgresRepository) UpdateProject(ctx context.Context, req UpdateProjectRequest) (ProjectResponse, error) {
	params := gvdb.UpdateProjectParams{ID: req.ID}
	if req.Name != nil {
		params.SetName = true
		params.Name = *req.Name
	}
	if req.Description != nil {
		params.SetDescription = true
		params.Description = *req.Description
	}
	if req.DueAt.Set {
		if req.DueAt.Value == nil {
			params.ClearDueAt = true
		} else {
			params.SetDueAt = true
			params.DueAt = *req.DueAt.Value
		}
	}
	if req.ParentID != nil {
		params.SetParentID = true
		params.ParentID = *req.ParentID
	}
	if req.StartedAt.Set {
		if req.StartedAt.Value == nil {
			params.ClearStartedAt = true
		} else {
			params.SetStartedAt = true
			params.StartedAt = pgtype.Timestamptz{Time: *req.StartedAt.Value, Valid: true}
		}
	}
	if req.FinishedAt.Set {
		if req.FinishedAt.Value == nil {
			params.ClearFinishedAt = true
		} else {
			params.SetFinishedAt = true
			params.FinishedAt = pgtype.Timestamptz{Time: *req.FinishedAt.Value, Valid: true}
		}
	}

	row, err := r.q.UpdateProject(ctx, params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProjectResponse{}, ErrNotFound
		}
		return ProjectResponse{}, err
	}

	return ProjectResponse{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       pgconv.DatePtr(row.DueAt),
		ParentID:    row.ParentID,
		StartedAt:   pgconv.TimePtr(row.StartedAt),
		FinishedAt:  pgconv.TimePtr(row.FinishedAt),
	}, nil
}

func (r *PostgresRepository) GetActiveProjects(ctx context.Context) ([]ActiveProject, error) {
	rows, err := r.q.GetActiveProjects(ctx)
	if err != nil {
		return nil, err
	}

	projects := make([]ActiveProject, len(rows))
	for i, row := range rows {
		projects[i] = ActiveProject{
			ID:       row.ID,
			ParentID: row.ParentID,
			Name:     row.Name,
			DueAt:    pgconv.DatePtr(row.DueAt),
		}
	}
	return projects, nil
}

func (r *PostgresRepository) GetProject(ctx context.Context, id int32) (ProjectDetailResponse, error) {
	row, err := r.q.GetProjectByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProjectDetailResponse{}, ErrNotFound
		}
		return ProjectDetailResponse{}, err
	}
	return ProjectDetailResponse{
		ID:          row.ID,
		ParentID:    row.ParentID,
		Name:        row.Name,
		Description: row.Description,
		DueAt:       pgconv.DatePtr(row.DueAt),
		StartedAt:   pgconv.TimePtr(row.StartedAt),
		FinishedAt:  pgconv.TimePtr(row.FinishedAt),
		TimeSpent:   row.TimeSpent,
	}, nil
}

func (r *PostgresRepository) GetProjectChildren(ctx context.Context, projectID int32) (ProjectChildrenResponse, error) {
	descendants, err := r.q.GetProjectWithDescendants(ctx, projectID)
	if err != nil {
		return ProjectChildrenResponse{}, err
	}
	if len(descendants) == 0 {
		return ProjectChildrenResponse{}, ErrNotFound
	}

	projectIDs := make([]int32, len(descendants))
	for i, d := range descendants {
		projectIDs[i] = d.ID
	}

	taskRows, err := r.q.GetTasksByProjectIDs(ctx, projectIDs)
	if err != nil {
		return ProjectChildrenResponse{}, err
	}

	projectTasks := make(map[int32][]ProjectChildNode)
	for _, row := range taskRows {
		dependsOn, err := unmarshalDepRefs(row.DependsOn)
		if err != nil {
			return ProjectChildrenResponse{}, err
		}
		blocks, err := unmarshalDepRefs(row.Blocks)
		if err != nil {
			return ProjectChildrenResponse{}, err
		}
		blocked := row.Blocked
		priority := row.Priority
		taskType := row.TaskType
		node := ProjectChildNode{
			ID:          row.ID,
			Type:        "task",
			Name:        row.Name,
			Description: row.Description,
			DueAt:       pgconv.DatePtr(row.DueAt),
			StartedAt:   pgconv.TimePtr(row.StartedAt),
			FinishedAt:  pgconv.TimePtr(row.FinishedAt),
			TimeSpent:   row.TimeSpent,
			ProjectID:   row.ProjectID,
			TaskType:    &taskType,
			Recurrence:  row.Recurrence,
			Priority:    &priority,
			DependsOn:   dependsOn,
			Blocks:      blocks,
			Blocked:     &blocked,
			Todos:       unmarshalTodos(row.Todos, row.ID),
		}
		if row.ProjectID != nil {
			projectTasks[*row.ProjectID] = append(projectTasks[*row.ProjectID], node)
		}
	}

	root := descendants[0]
	project := ProjectDetailResponse{
		ID:          root.ID,
		ParentID:    root.ParentID,
		Name:        root.Name,
		Description: root.Description,
		DueAt:       pgconv.DatePtr(root.DueAt),
		StartedAt:   pgconv.TimePtr(root.StartedAt),
		FinishedAt:  pgconv.TimePtr(root.FinishedAt),
		TimeSpent:   root.TimeSpent,
	}

	var children []ProjectChildNode
	for _, d := range descendants[1:] {
		if d.ParentID != nil && *d.ParentID == projectID {
			children = append(children, ProjectChildNode{
				ID:          d.ID,
				Type:        "project",
				Name:        d.Name,
				Description: d.Description,
				DueAt:       pgconv.DatePtr(d.DueAt),
				StartedAt:   pgconv.TimePtr(d.StartedAt),
				FinishedAt:  pgconv.TimePtr(d.FinishedAt),
				TimeSpent:   d.TimeSpent,
				ParentID:    d.ParentID,
			})
		}
	}
	children = append(children, topoSortByDeps(projectTasks[projectID])...)

	// Order children by status group. Stable sort keeps the within-group
	// ordering intact: natural order for sub-projects, dependency order
	// (topoSortByDeps) for tasks.
	sort.SliceStable(children, func(i, j int) bool {
		return childStatusRank(children[i]) < childStatusRank(children[j])
	})

	if children == nil {
		children = []ProjectChildNode{}
	}

	return ProjectChildrenResponse{
		Project:  project,
		Children: children,
	}, nil
}

// childStatusRank returns the display-ordering rank of a project child by its
// status group. Lower ranks sort first. The order is:
//
//	0 projects in progress   1 projects not started
//	2 task continuous        3 task recurring        4 task in progress
//	5 task to do             6 projects completed     7 tasks completed
func childStatusRank(c ProjectChildNode) int {
	completed := c.FinishedAt != nil
	started := c.StartedAt != nil

	if c.Type == "project" {
		switch {
		case completed:
			return 6
		case started:
			return 0
		default:
			return 1
		}
	}

	// task
	switch {
	case completed:
		return 7
	case !started:
		return 5
	}
	taskType := ""
	if c.TaskType != nil {
		taskType = *c.TaskType
	}
	switch taskType {
	case "continuous":
		return 2
	case "recurring":
		return 3
	default:
		return 4
	}
}

// topoSortByDeps reorders a project's task list so that any task whose
// DependsOn references another task in the same list comes after that
// dependency. The pre-existing relative order is preserved as the tiebreaker
// (Kahn's algorithm with a min-heap keyed by original index).
func topoSortByDeps(nodes []ProjectChildNode) []ProjectChildNode {
	n := len(nodes)
	if n <= 1 {
		return nodes
	}
	idx := make(map[int32]int, n)
	for i, t := range nodes {
		idx[t.ID] = i
	}
	inDeg := make([]int, n)
	blockedBy := make([][]int, n)
	for i, t := range nodes {
		for _, d := range t.DependsOn {
			j, ok := idx[d.ID]
			if !ok {
				continue
			}
			inDeg[i]++
			blockedBy[j] = append(blockedBy[j], i)
		}
	}
	ready := make([]int, 0, n)
	for i, deg := range inDeg {
		if deg == 0 {
			ready = append(ready, i)
		}
	}
	sort.Ints(ready)
	out := make([]ProjectChildNode, 0, n)
	for len(ready) > 0 {
		cur := ready[0]
		ready = ready[1:]
		out = append(out, nodes[cur])
		for _, j := range blockedBy[cur] {
			inDeg[j]--
			if inDeg[j] == 0 {
				pos := sort.SearchInts(ready, j)
				ready = append(ready, 0)
				copy(ready[pos+1:], ready[pos:])
				ready[pos] = j
			}
		}
	}
	if len(out) != n {
		return nodes
	}
	return out
}

func (r *PostgresRepository) FinishDescendantProjects(ctx context.Context, projectID int32) error {
	return r.q.FinishDescendantProjects(ctx, projectID)
}

func (r *PostgresRepository) DeleteProject(ctx context.Context, id int32) error {
	return r.deleteByID(ctx, "projects", id)
}

func (r *PostgresRepository) ListProjectsFast(ctx context.Context) ([]ProjectFastResponse, error) {
	rows, err := r.q.ListProjectsFast(ctx)
	if err != nil {
		return nil, err
	}

	projects := make([]ProjectFastResponse, len(rows))
	for i, row := range rows {
		projects[i] = ProjectFastResponse{
			ID:   row.ID,
			Name: row.Name,
		}
	}

	return projects, nil
}

func (r *PostgresRepository) GetRootProjects(ctx context.Context) ([]ProjectResponse, error) {
	rows, err := r.q.GetRootProjects(ctx)
	if err != nil {
		return nil, err
	}

	projects := make([]ProjectResponse, len(rows))
	for i, row := range rows {
		projects[i] = ProjectResponse{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
			DueAt:       pgconv.DatePtr(row.DueAt),
			ParentID:    row.ParentID,
		}
	}

	return projects, nil
}
