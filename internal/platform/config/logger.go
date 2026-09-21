package config

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
)

// SetupLogger installs slog from LOG_FILE / LOG_FORMAT.
// Log level is fixed at info (not env-configurable).
// LOG_FILE=- means stdout only. Returns a closer for the optional log file.
func (c Config) SetupLogger() (func() error, error) {
	filePath := c.LogFile
	if filePath == "" {
		filePath = "app.log"
	}
	format := c.LogFormat
	if format == "" {
		format = "text"
	}

	writers := []io.Writer{os.Stdout}
	var file *os.File
	if filePath != "-" {
		f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, fmt.Errorf("open log file %q: %w", filePath, err)
		}
		file = f
		writers = append(writers, f)
	}

	handlerOpts := &slog.HandlerOptions{Level: slog.LevelInfo}
	w := io.MultiWriter(writers...)

	var handler slog.Handler
	if strings.EqualFold(format, "json") {
		handler = slog.NewJSONHandler(w, handlerOpts)
	} else {
		handler = slog.NewTextHandler(w, handlerOpts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	log.SetFlags(0)
	log.SetOutput(slog.NewLogLogger(logger.Handler(), slog.LevelInfo).Writer())

	return func() error {
		if file == nil {
			return nil
		}
		return file.Close()
	}, nil
}
