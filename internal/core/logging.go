package core

import (
	"log/slog"
	"os"
)

func SetupLogging() {
	slog.SetDefault(slog.New(LogHandler{Handler: slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})}))
}
