package backup

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Dumper interface {
	Dump(ctx context.Context, w io.Writer) error
}

type Locker interface {
	TryLock(ctx context.Context) (unlock func(), err error)
}

type Uploader interface {
	Upload(ctx context.Context, key, file string) error
}

type Config struct {
	Dir        string
	KeepHourly time.Duration
	KeepDaily  time.Duration
}

const (
	nameLayout = "20060102T150405Z"
	dayLayout  = "20060102"
	hourlyDir  = "hourly"
	dailyDir   = "daily"
)

var namePattern = regexp.MustCompile(`^gv-db-\d{8}T\d{6}Z\.sql\.gz$`)

type Service struct {
	dumper   Dumper
	locker   Locker
	uploader Uploader
	cfg      Config
}

func NewService(dumper Dumper, locker Locker, uploader Uploader, cfg Config) *Service {
	return &Service{dumper: dumper, locker: locker, uploader: uploader, cfg: cfg}
}

func (s *Service) Run(ctx context.Context) (*Backup, error) {
	unlock, err := s.locker.TryLock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()

	for _, dir := range []string{hourlyDir, dailyDir} {
		if err := os.MkdirAll(filepath.Join(s.cfg.Dir, dir), 0o750); err != nil {
			return nil, fmt.Errorf("create %s dir: %w", dir, err)
		}
	}

	name := "gv-db-" + time.Now().UTC().Format(nameLayout) + ".sql.gz"
	if b, err := s.stat(hourlyDir, name); err == nil {
		return b, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err := s.write(ctx, name); err != nil {
		return nil, err
	}

	b, err := s.stat(hourlyDir, name)
	if err != nil {
		return nil, err
	}

	daily, err := s.promote(b)
	if err != nil {
		slog.WarnContext(ctx, "backup: creating daily copy", "error", err)
	}
	if s.uploader != nil {
		if err := s.upload(ctx, b.Name, daily); err != nil {
			slog.ErrorContext(ctx, "backup: s3 upload failed", "error", err)
		}
	}

	if err := s.prune(); err != nil {
		slog.WarnContext(ctx, "backup: pruning old backups", "error", err)
	}
	return b, nil
}

func (s *Service) write(ctx context.Context, name string) (err error) {
	tmp, err := os.CreateTemp(filepath.Join(s.cfg.Dir, hourlyDir), ".gv-db-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()

	gz := gzip.NewWriter(tmp)
	if err = s.dumper.Dump(ctx, gz); err != nil {
		return err
	}
	if err = gz.Close(); err != nil {
		return fmt.Errorf("compress: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	if err = os.Rename(tmp.Name(), s.path(hourlyDir, name)); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

func (s *Service) promote(b *Backup) (bool, error) {
	sameDay, err := filepath.Glob(s.path(dailyDir, "gv-db-"+b.CreatedAt.Format(dayLayout)+"T*Z.sql.gz"))
	if err != nil {
		return false, err
	}
	if len(sameDay) > 0 {
		return false, nil
	}
	if err := os.Link(s.path(hourlyDir, b.Name), s.path(dailyDir, b.Name)); err != nil {
		return false, fmt.Errorf("link: %w", err)
	}
	return true, nil
}

func (s *Service) prune() error {
	now := time.Now()
	for _, tier := range []struct {
		dir  string
		keep time.Duration
	}{{hourlyDir, s.cfg.KeepHourly}, {dailyDir, s.cfg.KeepDaily}} {
		backups, err := s.list(tier.dir)
		if err != nil {
			return err
		}
		for i, b := range backups {
			if i == 0 || now.Sub(b.CreatedAt) < tier.keep {
				continue
			}
			if err := os.Remove(s.path(tier.dir, b.Name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) List() ([]Backup, error) {
	return s.list(hourlyDir, dailyDir)
}

func (s *Service) list(dirs ...string) ([]Backup, error) {
	seen := map[string]bool{}
	backups := []Backup{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(filepath.Join(s.cfg.Dir, dir))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !namePattern.MatchString(e.Name()) || seen[e.Name()] {
				continue
			}
			seen[e.Name()] = true
			b, err := s.stat(dir, e.Name())
			if err != nil {
				return nil, err
			}
			backups = append(backups, *b)
		}
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].Name > backups[j].Name })
	return backups, nil
}

func (s *Service) stat(dir, name string) (*Backup, error) {
	info, err := os.Stat(s.path(dir, name))
	if err != nil {
		return nil, err
	}
	created, err := time.Parse(nameLayout, strings.TrimSuffix(strings.TrimPrefix(name, "gv-db-"), ".sql.gz"))
	if err != nil {
		return nil, err
	}
	return &Backup{Name: name, Size: info.Size(), CreatedAt: created}, nil
}

func (s *Service) path(dir, name string) string {
	return filepath.Join(s.cfg.Dir, dir, name)
}

func (s *Service) Path(name string) (string, error) {
	if !namePattern.MatchString(name) {
		return "", ErrInvalidName
	}
	for _, dir := range []string{hourlyDir, dailyDir} {
		path := s.path(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	return "", ErrNotFound
}

func (s *Service) upload(ctx context.Context, name string, daily bool) error {
	if err := s.uploader.Upload(ctx, hourlyDir+"/"+name, s.path(hourlyDir, name)); err != nil {
		return err
	}
	if daily {
		return s.uploader.Upload(ctx, dailyDir+"/"+name, s.path(dailyDir, name))
	}
	return nil
}
