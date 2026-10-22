// Package restore replaces the home database with the one restored from EC2 and reports what home
// wrote since the dump both started from. It works on local dump files and never touches S3.
package restore

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gv-api/internal/backup"
	"gv-api/internal/report"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrBadDump = errors.New("dump is not a readable gzip file or is empty")

type Config struct {
	// Admin is connected to a database other than Home, so Home can be renamed.
	Admin *pgxpool.Pool
	// URL builds the connection string of a database on the same server.
	URL func(db string) string
	// Home is the live database name.
	Home string
	// Dir keeps the pre-restore dumps and the done markers.
	Dir  string
	Psql string
	// Ignore lists, per table, the columns a trigger derives; the report does not compare them.
	Ignore map[string][]string
	// Release closes whatever the application holds open on Home; it runs right before the swap.
	Release func()
}

type Request struct {
	// ID names this restore (the failback dump's key); it keys the pre-restore dump and the done marker.
	ID   string
	Base string
	EC2  string
}

type Outcome struct {
	// Report is nil when the restore had already been done.
	Report      *report.Report
	Swapped     bool
	AlreadyDone bool
	PreRestore  string
}

type Runner struct {
	cfg Config
}

func New(cfg Config) *Runner {
	if cfg.Psql == "" {
		cfg.Psql = "psql"
	}
	return &Runner{cfg: cfg}
}

func (r *Runner) baseDB() string { return r.cfg.Home + "_restore_base" }
func (r *Runner) ec2DB() string  { return r.cfg.Home + "_restore_ec2" }
func (r *Runner) oldDB() string  { return r.cfg.Home + "_pre_restore" }

// Run loads both dumps into scratch databases, reports what home holds that the EC2 dump does not,
// and swaps the EC2 database into place. It starts from scratch every time, so it can be run
// again after a crash. A report that cannot be built is recorded in the report, never fatal.
func (r *Runner) Run(ctx context.Context, req Request) (*Outcome, error) {
	marker := filepath.Join(r.cfg.Dir, "swapped-"+req.ID)
	if _, err := os.Stat(marker); err == nil {
		return &Outcome{Swapped: true, AlreadyDone: true}, nil
	}
	for _, file := range []string{req.Base, req.EC2} {
		if err := verify(file); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
	}

	out := &Outcome{}
	var err error
	if out.PreRestore, err = r.preRestore(ctx, req.ID); err != nil {
		return nil, fmt.Errorf("pre-restore dump: %w", err)
	}
	if err := r.load(ctx, r.baseDB(), req.Base); err != nil {
		return nil, err
	}
	if err := r.load(ctx, r.ec2DB(), req.EC2); err != nil {
		return nil, err
	}

	out.Report = r.report(ctx)
	if r.cfg.Release != nil {
		r.cfg.Release()
	}
	if err := r.swap(ctx); err != nil {
		return nil, fmt.Errorf("swap: %w", err)
	}
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		return nil, err
	}
	out.Swapped = true
	return out, nil
}

func (r *Runner) report(ctx context.Context) *report.Report {
	rep, err := r.build(ctx)
	if err != nil {
		slog.Error("restore: cannot build the report", "error", err)
		return &report.Report{At: time.Now().UTC(), Items: []report.Item{}, Error: err.Error()}
	}
	return rep
}

func (r *Runner) build(ctx context.Context) (*report.Report, error) {
	pools, err := r.open(ctx, r.baseDB(), r.cfg.Home, r.ec2DB())
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, p := range pools {
			p.Close()
		}
	}()
	return report.Build(ctx, pools[0], pools[1], pools[2], r.cfg.Ignore)
}

// verify reads the whole gzip stream, so a truncated download is caught before anything changes.
func verify(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return ErrBadDump
	}
	n, err := io.Copy(io.Discard, gz)
	if err != nil || n == 0 {
		return ErrBadDump
	}
	return nil
}

func (r *Runner) preRestore(ctx context.Context, id string) (string, error) {
	if err := os.MkdirAll(r.cfg.Dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(r.cfg.Dir, "pre-restore-"+id+".sql.gz")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}

	tmp := path + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp)

	gz := gzip.NewWriter(f)
	if err := (backup.PgDump{URL: r.cfg.URL(r.cfg.Home)}).Dump(ctx, gz); err != nil {
		f.Close()
		return "", err
	}
	if err := gz.Close(); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := verify(tmp); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}

func (r *Runner) load(ctx context.Context, db, dump string) error {
	if err := r.recreate(ctx, db); err != nil {
		return err
	}
	f, err := os.Open(dump)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return ErrBadDump
	}

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, r.cfg.Psql, "-X", "-q", "-v", "ON_ERROR_STOP=1", "--dbname", r.cfg.URL(db))
	cmd.Stdin = gz
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("load %s into %s: %w: %s", filepath.Base(dump), db, err, bytes.TrimSpace(stderr.Bytes()))
	}
	return nil
}

func (r *Runner) recreate(ctx context.Context, db string) error {
	id := pgx.Identifier{db}.Sanitize()
	if _, err := r.cfg.Admin.Exec(ctx, "DROP DATABASE IF EXISTS "+id+" WITH (FORCE)"); err != nil {
		return err
	}
	_, err := r.cfg.Admin.Exec(ctx, "CREATE DATABASE "+id)
	return err
}

func (r *Runner) open(ctx context.Context, dbs ...string) ([]*pgxpool.Pool, error) {
	var pools []*pgxpool.Pool
	for _, db := range dbs {
		p, err := pgxpool.New(ctx, r.cfg.URL(db))
		if err != nil {
			for _, open := range pools {
				open.Close()
			}
			return nil, err
		}
		pools = append(pools, p)
	}
	return pools, nil
}

// swap renames the live database out of the way and the merged one into its place.
//
// New connections to home are refused first: the app's pool reconnects the moment its sessions are
// terminated, and a rename fails while any session is open. A failure puts everything back.
func (r *Runner) swap(ctx context.Context) error {
	home, merged, old := pgx.Identifier{r.cfg.Home}.Sanitize(), pgx.Identifier{r.ec2DB()}.Sanitize(), pgx.Identifier{r.oldDB()}.Sanitize()
	exec := func(sql string, args ...any) error {
		_, err := r.cfg.Admin.Exec(ctx, sql, args...)
		return err
	}
	undo := context.WithoutCancel(ctx)

	if err := exec("ALTER DATABASE " + home + " ALLOW_CONNECTIONS false"); err != nil {
		return err
	}
	err := exec("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = ANY($1) AND pid <> pg_backend_pid()",
		[]string{r.cfg.Home, r.ec2DB(), r.baseDB()})
	if err == nil {
		err = exec("DROP DATABASE IF EXISTS " + old + " WITH (FORCE)")
	}
	if err == nil {
		err = exec("ALTER DATABASE " + home + " RENAME TO " + old)
	}
	if err != nil {
		_, _ = r.cfg.Admin.Exec(undo, "ALTER DATABASE "+home+" ALLOW_CONNECTIONS true")
		return err
	}

	if err := exec("ALTER DATABASE " + merged + " RENAME TO " + home); err != nil {
		_, _ = r.cfg.Admin.Exec(undo, "ALTER DATABASE "+old+" RENAME TO "+home)
		_, _ = r.cfg.Admin.Exec(undo, "ALTER DATABASE "+home+" ALLOW_CONNECTIONS true")
		return err
	}
	if err := exec("ALTER DATABASE " + old + " ALLOW_CONNECTIONS true"); err != nil {
		return err
	}
	return exec("DROP DATABASE IF EXISTS " + pgx.Identifier{r.baseDB()}.Sanitize() + " WITH (FORCE)")
}
