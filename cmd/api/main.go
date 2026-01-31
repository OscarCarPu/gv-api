package main

import (
	"context"
	"log"
	"net/http"

	"gv-api/internal/config"
	"gv-api/internal/database"
	"gv-api/internal/habits"

	"github.com/go-chi/chi/v5"
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

	habitRepo := habits.NewRepository(db)
	habitService := habits.NewService(habitRepo)
	habitHandler := habits.NewHandler(habitService)

	r := chi.NewRouter()
	r.Get("/habits", habitHandler.GetDaily)
	r.Post("/habits/log", habitHandler.UpsertLog)

	log.Printf("Starting server on port %s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, r))
}
