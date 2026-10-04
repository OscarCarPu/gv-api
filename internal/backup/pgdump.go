package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type PgDump struct {
	URL string
}

func (p PgDump) Dump(ctx context.Context, w io.Writer) error {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "pg_dump",
		"--clean", "--if-exists", "--no-owner", "--no-privileges", "--dbname", p.URL)
	cmd.Stdout = w
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_dump: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
