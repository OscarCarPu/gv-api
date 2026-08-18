# gv-api — Issues abiertos

## Tareas con un proyecto no activo desaparecen de `GetActiveTree`

**Fichero:** `internal/tasks/service.go:165-171`

Cuando una tarea tiene un `project_id` que no está en el conjunto de proyectos
activos (porque su proyecto se terminó), cae en el `continue` y no se adjunta a
ningún nodo ni pasa a huérfanas: desaparece de la respuesta sin señal alguna.
Una tarea sin terminar se vuelve invisible.

```go
if t.ProjectID != nil {
	if _, ok := projectNodes[*t.ProjectID]; ok {
		projectTasks[*t.ProjectID] = append(projectTasks[*t.ProjectID], node)
	}
	// project not active — skip task
	continue
}
orphanTasks = append(orphanTasks, node)
```

Arreglo: tratarla como huérfana en vez de descartarla.

```go
if t.ProjectID != nil {
	if _, ok := projectNodes[*t.ProjectID]; ok {
		projectTasks[*t.ProjectID] = append(projectTasks[*t.ProjectID], node)
		continue
	}
	// project_id puesto pero proyecto no activo — sale como huérfana
}
orphanTasks = append(orphanTasks, node)
```
