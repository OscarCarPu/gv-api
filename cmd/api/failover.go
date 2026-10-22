package main

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"gv-api/internal/config"
	"gv-api/internal/database"
	"gv-api/internal/failover"
	"gv-api/internal/restore"

	"github.com/jackc/pgx/v5/pgxpool"
)

// newFailover builds home's side of the failover and reads the AWS state once. It returns nil when
// this is not home or no token is set. resetPool closes the app's idle connections, so they
// reconnect to the database that was swapped into place.
func newFailover(ctx context.Context, cfg *config.Config, resetPool func()) (*failover.Controller, error) {
	if cfg.FailoverSide != "home" || cfg.FailoverToken == "" {
		return nil, nil
	}
	if cfg.BackupS3Bucket == "" {
		return nil, fmt.Errorf("FAILOVER_TOKEN is set but BACKUP_S3_BUCKET is not")
	}

	u, err := url.Parse(cfg.DBUrl)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	home := strings.TrimPrefix(u.Path, "/")
	urlFor := func(db string) string {
		c := *u
		c.Path = "/" + db
		return c.String()
	}

	admin, err := pgxpool.New(ctx, urlFor("postgres"))
	if err != nil {
		return nil, err
	}
	params, err := failover.NewSSM(ctx)
	if err != nil {
		return nil, err
	}
	store, err := failover.NewS3(ctx, cfg.BackupS3Bucket)
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(cfg.BackupDir, "failover")
	ctl := failover.New(failover.Options{
		Token:  cfg.FailoverToken,
		Params: params,
		Store:  store,
		Dir:    dir,
		Restorer: restore.New(restore.Config{
			Admin:   admin,
			URL:     urlFor,
			Home:    home,
			Dir:     dir,
			Ignore:  map[string][]string{"accounts": {"total"}},
			Release: resetPool,
		}),
		After: func() {
			resetPool()
			_ = database.Migrate(cfg.DBUrl, "db/migrations")
		},
	})
	ctl.Start(ctx)
	return ctl, nil
}
