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
	"gv-api/internal/database/sqlc"
	"gv-api/internal/habits"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config", err)
	}

	db, err := database.New(context.Background(), cfg.DBUrl)
	if err != nil {
		log.Fatal(err)
	}

	//nolint:errcheck // if the db is closed, the program has already exited
	defer db.Close()

	queries := sqlc.New(db)

	// Habit Setup
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		log.Fatalf("Failed to load timezone %q: %v", cfg.Timezone, err)
	}

	habitRepo := habits.NewRepository(queries)
	habitService := habits.NewService(habitRepo, loc)
	habitHandler := habits.NewHandler(habitService)

	// Auth Setup
	authService := auth.NewService(cfg, nil)
	authHandler := auth.NewHandler(authService)
	authMiddleware := auth.NewMiddleware(authService)

	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization"},
	}))

	// Public
	r.Post("/login", authHandler.Login)
	r.Post("/login/2fa", authHandler.Login2FA)

	// Protected
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware.Handle)
		r.Get("/habits", habitHandler.GetDaily)
		r.Post("/habits", habitHandler.CreateHabit)
		r.Post("/habits/log", habitHandler.UpsertLog)
	})

	log.Printf("Starting server on port %s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, r))
}
