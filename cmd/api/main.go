package main

import (
	"context"
	"log"
	"net/http"
	"time"
	_ "time/tzdata"

	"gv-api/internal/auth"
	"gv-api/internal/config"
	"gv-api/internal/database"
	"gv-api/internal/database/habitsdb"
	"gv-api/internal/database/tasksdb"
	"gv-api/internal/database/varietiesdb"
	"gv-api/internal/habits"
	"gv-api/internal/tasks"
	"gv-api/internal/varieties"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config", err)
	}

	if err := database.Migrate(cfg.DBUrl, "db/migrations"); err != nil {
		log.Fatal("Failed to run migrations: ", err)
	}

	db, err := database.New(context.Background(), cfg.DBUrl)
	if err != nil {
		log.Fatal(err)
	}

	//nolint:errcheck // if the db is closed, the program has already exited
	defer db.Close()

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatalf("Failed to load timezone %q: %v", cfg.Timezone, err)
	}

	// Habit Setup
	habitQueries := habitsdb.New(db)
	habitRepo := habits.NewRepository(habitQueries)
	habitService := habits.NewService(habitRepo, loc)
	habitHandler := habits.NewHandler(habitService)

	// Tasks Setup
	taskQueries := tasksdb.New(db)
	taskRepo := tasks.NewRepository(taskQueries)
	taskService := tasks.NewService(taskRepo, loc)
	taskHandler := tasks.NewHandler(taskService)

	// Varieties Setup
	varietyQueries := varietiesdb.New(db)
	varietyRepo := varieties.NewRepository(varietyQueries)
	varietyService := varieties.NewService(varietyRepo)
	varietyHandler := varieties.NewHandler(varietyService)

	// Auth Setup
	authService := auth.NewService(cfg, nil)
	authHandler := auth.NewHandler(authService)
	fullMiddleware := auth.NewMiddleware(authService, "full")
	semiMiddleware := auth.NewMiddleware(authService, "semi", "full")

	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		ExposedHeaders:   []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Public
	r.Post("/login", authHandler.Login)
	r.Post("/login/2fa", authHandler.Login2FA)

	// Semiprivate (semi or full token)
	r.Group(func(r chi.Router) {
		r.Use(semiMiddleware.Handle)
		r.Get("/varieties", varietyHandler.List)
		r.Get("/varieties/{id}", varietyHandler.Get)
		r.Post("/varieties", varietyHandler.Create)
		r.Put("/varieties/{id}", varietyHandler.Update)
		r.Delete("/varieties/{id}", varietyHandler.Delete)
	})

	// Full private
	r.Group(func(r chi.Router) {
		r.Use(fullMiddleware.Handle)
		r.Get("/habits", habitHandler.GetDaily)
		r.Post("/habits", habitHandler.CreateHabit)
		r.Post("/habits/log", habitHandler.UpsertLog)
		r.Get("/tasks/tree", taskHandler.GetActiveTree)
		r.Get("/tasks/projects", taskHandler.GetRootProjects)
		r.Get("/tasks/projects/list-fast", taskHandler.ListProjectsFast)
		r.Get("/tasks/projects/{id}", taskHandler.GetProject)
		r.Get("/tasks/projects/{id}/children", taskHandler.GetProjectChildren)
		r.Post("/tasks/projects", taskHandler.CreateProject)
		r.Get("/tasks/tasks/by-due-date", taskHandler.GetTasksByDueDate)
		r.Get("/tasks/tasks/list-fast", taskHandler.ListTasksFast)
		r.Post("/tasks/tasks", taskHandler.CreateTask)
		r.Post("/tasks/todos", taskHandler.CreateTodo)
		r.Patch("/tasks/todos/{id}", taskHandler.UpdateTodo)
		r.Get("/tasks/time-entries", taskHandler.GetTimeEntriesByDateRange)
		r.Get("/tasks/time-entries/active", taskHandler.GetActiveTimeEntry)
		r.Get("/tasks/time-entries/summary", taskHandler.GetTimeEntrySummary)
		r.Get("/tasks/time-entries/history", taskHandler.GetTimeEntryHistory)
		r.Post("/tasks/time-entries", taskHandler.CreateTimeEntry)
		r.Patch("/tasks/time-entries/{id}", taskHandler.UpdateTimeEntry)
		r.Get("/tasks/tasks/{id}", taskHandler.GetTask)
		r.Get("/tasks/tasks/{id}/time-entries", taskHandler.GetTaskTimeEntries)
		r.Patch("/tasks/tasks/{id}", taskHandler.UpdateTask)
		r.Patch("/tasks/projects/{id}", taskHandler.UpdateProject)
		r.Delete("/tasks/projects/{id}", taskHandler.DeleteProject)
		r.Delete("/tasks/tasks/{id}", taskHandler.DeleteTask)
		r.Delete("/tasks/todos/{id}", taskHandler.DeleteTodo)
		r.Delete("/tasks/time-entries/{id}", taskHandler.DeleteTimeEntry)
		r.Get("/habits/{id}/history", habitHandler.GetHistory)
		r.Delete("/habits/{id}", habitHandler.DeleteHabit)
	})

	log.Printf("Starting server on port %s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, r))
}
