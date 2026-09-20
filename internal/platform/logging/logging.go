package logging

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
)

// Options configures dual stdout + file logging.
type Options struct {
	// File path for the app log (default app.log). Empty disables file logging.
	File string
	// Level: debug, info, warn, error (default info).
	Level string
	// Format: text or json (default text). Use json in prod if you prefer.
	Format string
}

// Setup installs the process-wide slog default logger writing to stdout and
// optionally to File (append). It also bridges the stdlib log package into slog
// so existing log.Printf calls land in the same sinks.
//
// Call the returned closer on shutdown to flush/close the file handle.
func Setup(opts Options) (func() error, error) {
	if opts.File == "" {
		opts.File = "app.log"
	}
	if opts.Level == "" {
		opts.Level = "info"
	}
	if opts.Format == "" {
		opts.Format = "text"
	}

	level := parseLevel(opts.Level)

	writers := []io.Writer{os.Stdout}
	var file *os.File
	if opts.File != "-" {
		f, err := os.OpenFile(opts.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("open log file %q: %w", opts.File, err)
		}
		file = f
		writers = append(writers, f)
	}

	w := io.MultiWriter(writers...)
	handlerOpts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch strings.ToLower(opts.Format) {
	case "json":
		handler = slog.NewJSONHandler(w, handlerOpts)
	default:
		handler = slog.NewTextHandler(w, handlerOpts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Bridge classic log → slog so chi middleware / legacy calls share sinks.
	log.SetFlags(0)
	log.SetOutput(slog.NewLogLogger(logger.Handler(), slog.LevelInfo).Writer())

	closer := func() error {
		if file == nil {
			return nil
		}
		return file.Close()
	}
	return closer, nil
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
