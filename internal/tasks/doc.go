// Package tasks contains all the tasks for the GV API.
//
// Endpoints:
//
//	GET    /tasks/tree                          - active tree
//	GET    /tasks/projects                      - root projects
//	GET    /tasks/projects/list-fast            - flat list of active projects (id, name)
//	GET    /tasks/projects/{id}/children        - project children
//	GET    /tasks/projects/{id}/parent-candidates - valid new parents for a project
//	POST   /tasks/projects                      - create project
//	PATCH  /tasks/projects/{id}                 - update project (parent_id: id = move, null = root)
//	POST   /tasks/tasks                         - create task
//	PATCH  /tasks/tasks/{id}                    - update task
//	GET    /tasks/tasks/{id}/time-entries        - task time entries
//	POST   /tasks/todos                         - create todo
//	PATCH  /tasks/todos/{id}                    - update todo
//	POST   /tasks/time-entries                  - create time entry
//	PATCH  /tasks/time-entries/{id}             - update time entry
package tasks
