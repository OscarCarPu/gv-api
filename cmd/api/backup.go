package main

import (
	"context"
	"log/slog"
	"time"

	"gv-api/internal/backup"
	"gv-api/internal/config"
	"gv-api/internal/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newBackupService(cfg *config.Config, db *pgxpool.Pool) *backup.Service {
	return backup.NewService(
		backup.PgDump{URL: cfg.DBUrl},
		backup.NewPgLocker(db),
		backup.Config{Dir: cfg.BackupDir, KeepHourly: cfg.BackupKeepHourly, KeepDaily: cfg.BackupKeepDaily},
	)
}

func runBackup(cfg *config.Config) int {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	db, err := database.NewWithOptions(ctx, cfg.DBUrl, database.Options{MaxConns: 2, Ping: true})
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		return 1
	}
	defer db.Close()

	b, err := newBackupService(cfg, db).Run(ctx)
	if err != nil {
		slog.Error("backup failed", "error", err)
		return 1
	}
	slog.Info("backup done", "file", b.Name, "size", b.Size)
	return 0
}
