package main

import (
	"log"
	"net/http"

	"gv-api/internal/config"
	"gv-api/internal/database"
	"gv-api/internal/habits"

	"github.com/go-chi/chi/v5"
)

func main() {
	// 1. Config
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config", err)
	}

	// 2. Database
	db, err := database.New(cfg.DBUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// 3. Setup Layers
	// Note: If using sqlc, 'habitRepo' is auto-generated code
	habitRepo := habits.NewRepository(db)
	habitService := habits.NewService(habitRepo)
	habitHandler := habits.NewHandler(habitService)

	// 4. Routes
	r := chi.NewRouter()
	r.Get("/habits", habitHandler.GetDaily)
	r.Post("/habits/log", habitHandler.UpsertLog)

	// 5. Run
	log.Printf("Starting server on port %s", cfg.Port)
	http.ListenAndServe(":"+cfg.Port, r)
}
