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
	"gv-api/internal/backup"
	"gv-api/internal/calendar"
	calendargoogle "gv-api/internal/calendar/google"
	"gv-api/internal/capacity"
	"gv-api/internal/config"
	"gv-api/internal/core"
	"gv-api/internal/database"
	"gv-api/internal/finance"
	"gv-api/internal/habits"
	"gv-api/internal/lights"
	"gv-api/internal/pipeline"
	"gv-api/internal/plan"
	"gv-api/internal/printers"
	"gv-api/internal/rutas"
	"gv-api/internal/tasks"
	"gv-api/internal/uptime"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/shopspring/decimal"
)

func main() {
	core.SetupLogging()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if cfg.OnFailover() {
		slog.Warn("running on the backup server: lights, printers and uptime answer 503", "failover_side", cfg.FailoverSide)
	}

	if len(os.Args) > 1 && os.Args[1] == "backup" {
		os.Exit(runBackup(cfg))
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

	loc := cfg.Location

	habitRepo := habits.NewRepository(db)
	habitService := habits.NewService(habitRepo, loc)
	habitHandler := habits.NewHandler(habitService)

	taskRepo := tasks.NewRepository(db)
	taskService := tasks.NewService(taskRepo, loc)
	taskHandler := tasks.NewHandler(taskService)

	planRepo := plan.NewRepository(db)
	planService := plan.NewService(planRepo, taskService, loc)
	planHandler := plan.NewHandler(planService)

	financeRepo := finance.NewRepository(db)
	financeService := finance.NewService(financeRepo, loc)
	financeHandler := finance.NewHandler(financeService)

	lightsRepo := lights.NewRepository(db)
	var lightsDriver lights.Driver
	if cfg.LightsDriver == "bluez" && !cfg.OnFailover() {
		bluez := lights.NewBlueZDriver(cfg.LightsAdapter, cfg.LightsConnectTimeout, cfg.LightsIdleDisconnect)
		defer func() { _ = bluez.Close() }()
		lightsDriver = bluez
	} else {
		lightsDriver = lights.NewMockDriver()
	}
	lightsService := lights.NewService(lightsRepo, lightsDriver, cfg.LightsCacheTTL, cfg.LightsSettleAttempts, cfg.LightsSettleDelay)
	lightsHandler := lights.NewHandler(lightsService)

	printersService := printers.NewService(printers.Config{
		Printers: []printers.Printer{{
			ID:       "core-one",
			Name:     "Prusa CORE One",
			Model:    "Prusa CORE One + Buddy3D",
			RTSP:     cfg.PrinterRTSP,
			Host:     cfg.PrusaLinkHost,
			User:     cfg.PrusaLinkUser,
			Password: cfg.PrusaLinkPassword,
			APIKey:   cfg.PrusaLinkAPIKey,
			Storage:  cfg.PrusaLinkStorage,
		}},
		RecordingsDir: cfg.PrinterRecordingsDir,
		UploadsDir:    cfg.PrinterUploadsDir,
		MaxMinutes:    cfg.PrinterRecordingMaxMinutes,
		MaxBytes:      int64(cfg.PrinterRecordingsMaxGB * (1 << 30)),
		VideoCodec:    cfg.PrinterRecordingVideoCodec,
		Overlay:       cfg.PrinterRecordingOverlay,
		Font:          cfg.PrinterRecordingFont,
		SignKey:       []byte(cfg.JwtSecret),
	})
	printersHandler := printers.NewHandler(printersService)

	calendarRepo := calendar.NewRepository(db)
	calendarClient := calendargoogle.NewClient(calendargoogle.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  cfg.GoogleRedirectURL,
	})
	calendarService, err := calendar.NewService(calendarRepo, calendarClient, calendar.Config{
		ClientID:         cfg.GoogleClientID,
		ClientSecret:     cfg.GoogleClientSecret,
		RedirectURL:      cfg.GoogleRedirectURL,
		WebAppURL:        cfg.CalendarWebAppURL,
		WebhookEnabled:   cfg.CalendarWebhookEnabled,
		WebhookURL:       cfg.CalendarWebhookURL,
		WatchTTL:         cfg.CalendarWatchTTL,
		WatchRenewBefore: cfg.CalendarWatchRenewBefore,
		SyncInterval:     cfg.CalendarSyncInterval,
		Debounce:         cfg.CalendarDebounce,
		StateSecret:      []byte(cfg.JwtSecret),
		TokenKey:         cfg.GoogleTokenKey,
	}, loc, planService)
	if err != nil {
		slog.Error("failed to set up calendar", "error", err)
		os.Exit(1)
	}
	calendarHandler := calendar.NewHandler(calendarService)

	// tasks <-> plan depend on each other, so urgency providers are wired with a setter.
	capacityService := capacity.NewService(decimal.NewFromFloat(cfg.DailyCapacityHours), planService)
	capacityHandler := capacity.NewHandler(capacityService)
	taskService.SetUrgencyProviders(capacityService, planService)

	pipelineDB := &pipeline.DB{}
	if !cfg.OnFailover() {
		pipelineDB, err = pipeline.Connect(context.Background(), cfg.PipelineDBUrl)
		if err != nil {
			slog.Error("failed to configure pipeline database", "error", err)
			os.Exit(1)
		}
	}
	defer pipelineDB.Close()

	uptimeRepo := uptime.NewRepository(pipelineDB)
	uptimeHandler := uptime.NewHandler(uptime.NewService(uptimeRepo, cfg.PipelineStaleAfter))

	rutasRepo := rutas.NewRepository(db)
	rutasService := rutas.NewService(rutasRepo)
	rutasHandler := rutas.NewHandler(rutasService)

	backupService, err := newBackupService(context.Background(), cfg, db)
	if err != nil {
		slog.Error("failed to set up backups", "error", err)
		os.Exit(1)
	}
	backupHandler := backup.NewHandler(backupService)

	authService := auth.NewService(cfg, nil)
	authHandler := auth.NewHandler(authService)
	fullMiddleware := auth.NewMiddleware(authService, "full")
	semiMiddleware := auth.NewMiddleware(authService, "semi", "full")

	r := chi.NewRouter()
	r.Use(chimiddleware.Recoverer)
	r.Use(core.RequestID)
	r.Use(cors.Handler(core.CORSOptions(cfg.AllowedOrigins)))

	r.Get("/health", core.Health(db, cfg.HealthTimeout))

	// Public
	r.Post("/login", authHandler.Login)
	r.Post("/login/2fa", authHandler.Login2FA)
	calendarHandler.RegisterPublicRoutes(r)
	r.Group(func(r chi.Router) {
		if cfg.OnFailover() {
			r.Use(core.UnavailableOnFailover)
		}
		printersHandler.RegisterPublicRoutes(r)
	})

	// Semiprivate (semi or full token)
	r.Group(func(r chi.Router) {
		r.Use(semiMiddleware.Handle)
		rutasHandler.RegisterRoutes(r)

		r.Group(func(r chi.Router) {
			if cfg.OnFailover() {
				r.Use(core.UnavailableOnFailover)
			}
			lightsHandler.RegisterRoutes(r)
			printersHandler.RegisterRoutes(r)
			uptimeHandler.RegisterRoutes(r)
		})
	})

	// Full private
	r.Group(func(r chi.Router) {
		r.Use(fullMiddleware.Handle)
		habitHandler.RegisterRoutes(r)
		taskHandler.RegisterRoutes(r)
		planHandler.RegisterRoutes(r)
		financeHandler.RegisterRoutes(r)
		calendarHandler.RegisterRoutes(r)
		capacityHandler.RegisterRoutes(r)
		backupHandler.RegisterRoutes(r)
	})

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	go calendar.NewWorker(calendarService).Run(workerCtx)
	go backup.NewWorker(backupService, cfg.BackupInterval).Run(workerCtx)

	if !cfg.OnFailover() {
		lightsService.StartPolling(workerCtx, cfg.LightsPollInterval)
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
	stopWorker()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
}
