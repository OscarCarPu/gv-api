package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata"

	"gv-api/internal/auth"
	"gv-api/internal/config"
	"gv-api/internal/database"
	"gv-api/internal/finance"
	"gv-api/internal/habits"
	"gv-api/internal/lights"
	"gv-api/internal/middleware"
	"gv-api/internal/plan"
	"gv-api/internal/rutas"
	"gv-api/internal/tasks"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func main() {
	slog.SetDefault(slog.New(middleware.LogHandler{Handler: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})}))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if err := database.Migrate(cfg.DBUrl, "db/migrations"); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	db, err := database.New(context.Background(), cfg.DBUrl)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	//nolint:errcheck // if the db is closed, the program has already exited
	defer db.Close()

	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		slog.Error("failed to load timezone", "timezone", cfg.Timezone, "error", err)
		os.Exit(1)
	}

	// Habit Setup
	habitRepo := habits.NewRepository(db)
	habitService := habits.NewService(habitRepo, loc)
	habitHandler := habits.NewHandler(habitService)

	// Tasks Setup
	taskRepo := tasks.NewRepository(db)
	taskService := tasks.NewService(taskRepo, loc)
	taskHandler := tasks.NewHandler(taskService)

	// Plan Setup
	planRepo := plan.NewRepository(db)
	planService := plan.NewService(planRepo, taskService, loc)
	planHandler := plan.NewHandler(planService)

	// Finance Setup
	financeRepo := finance.NewRepository(db)
	financeService := finance.NewService(financeRepo, loc)
	financeHandler := finance.NewHandler(financeService)

	// Lights Setup
	// Which bulbs exist is a table; what they are doing comes from the bulbs themselves, over
	// this host's Bluetooth adapter.
	lightsRepo := lights.NewRepository(db)
	var lightsDriver lights.Driver
	if cfg.LightsDriver == "bluez" {
		bluez := lights.NewBlueZDriver(cfg.LightsAdapter, cfg.LightsConnectTimeout, cfg.LightsIdleDisconnect)
		defer func() { _ = bluez.Close() }()
		lightsDriver = bluez
	} else {
		lightsDriver = lights.NewMockDriver()
	}
	lightsHandler := lights.NewHandler(lights.NewService(lightsRepo, lightsDriver, cfg.LightsCacheTTL, cfg.LightsSettleAttempts, cfg.LightsSettleDelay))

	// Rutas Setup
	rutasRepo := rutas.NewRepository(db)
	rutasService := rutas.NewService(rutasRepo)
	rutasHandler := rutas.NewHandler(rutasService)

	// Auth Setup
	authService := auth.NewService(cfg, nil)
	authHandler := auth.NewHandler(authService)
	fullMiddleware := auth.NewMiddleware(authService, "full")
	semiMiddleware := auth.NewMiddleware(authService, "semi", "full")

	r := chi.NewRouter()
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "X-Request-ID"},
		ExposedHeaders:   []string{"Content-Length", "X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Public
	r.Post("/login", authHandler.Login)
	r.Post("/login/2fa", authHandler.Login2FA)

	// Semiprivate (semi or full token)
	r.Group(func(r chi.Router) {
		r.Use(semiMiddleware.Handle)
		lightsHandler.RegisterRoutes(r)
	})

	// Full private
	r.Group(func(r chi.Router) {
		r.Use(fullMiddleware.Handle)
		habitHandler.RegisterRoutes(r)
		taskHandler.RegisterRoutes(r)
		planHandler.RegisterRoutes(r)
		financeHandler.RegisterRoutes(r)
		rutasHandler.RegisterRoutes(r)
	})

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("shutting down server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}
