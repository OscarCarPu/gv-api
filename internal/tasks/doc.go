// Package tasks contains all the tasks for the GV API.
//
// Endpoints:
//
//	GET    /tasks/tree                          - active tree
//	GET    /tasks/projects                      - root projects
//	GET    /tasks/projects/{id}/children        - project children
//	POST   /tasks/projects                      - create project
//	PATCH  /tasks/projects/{id}                 - update project
//	POST   /tasks/tasks                         - create task
//	PATCH  /tasks/tasks/{id}                    - update task
//	GET    /tasks/tasks/{id}/time-entries        - task time entries
//	POST   /tasks/todos                         - create todo
//	PATCH  /tasks/todos/{id}                    - update todo
//	POST   /tasks/time-entries                  - create time entry
//	PATCH  /tasks/time-entries/{id}             - update time entry
package tasks
