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

func newBackupService(ctx context.Context, cfg *config.Config, db *pgxpool.Pool) (*backup.Service, error) {
	var uploader backup.Uploader
	if cfg.BackupS3Bucket != "" {
		u, err := backup.NewS3Uploader(ctx, cfg.BackupS3Bucket)
		if err != nil {
			return nil, err
		}
		uploader = u
	}
	return backup.NewService(
		backup.PgDump{URL: cfg.DBUrl},
		backup.NewPgLocker(db),
		uploader,
		backup.Config{Dir: cfg.BackupDir, KeepHourly: cfg.BackupKeepHourly, KeepDaily: cfg.BackupKeepDaily},
	), nil
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
	svc, err := newBackupService(ctx, cfg, db)
	if err != nil {
		slog.Error("failed to set up backups", "error", err)
		return 1
	}
	b, err := svc.Run(ctx)
	if err != nil {
		slog.Error("backup failed", "error", err)
		return 1
	}
	slog.Info("backup done", "file", b.Name, "size", b.Size)
	return 0
}
