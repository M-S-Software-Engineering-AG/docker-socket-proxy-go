package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"dockersocketproxy/internal/proxy"
)

const (
	defaultListen  = "0.0.0.0:2375"
	defaultSocket  = "/var/run/docker.sock"
	probeTimeout   = 10 * time.Second
	probeInterval  = 500 * time.Millisecond
	shutdownGrace  = 10 * time.Second
)

func main() {
	cfg := loadConfig()
	log := newLogger(cfg.LogLevel, cfg.LogFormat)

	logStartup(log, cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("socket.checking",
		slog.String("socket", cfg.DockerSocket),
		slog.Duration("timeout", probeTimeout),
	)
	apiVer, err := proxy.Probe(ctx, cfg.DockerSocket, probeTimeout, probeInterval, log)
	if err != nil {
		log.Error("socket.unavailable",
			slog.String("socket", cfg.DockerSocket),
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}
	log.Info("socket.ready",
		slog.String("socket", cfg.DockerSocket),
		slog.String("api_version", apiVer),
	)

	rcfg := &proxy.Config{Sections: cfg.Sections, Readonly: cfg.Readonly}
	h := proxy.NewHandler(rcfg, cfg.DockerSocket, log, cfg.LogPayload)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("proxy.listening",
			slog.String("addr", cfg.ListenAddr),
			slog.Bool("readonly", cfg.Readonly),
			slog.Bool("log_payload", cfg.LogPayload),
			slog.String("enabled", strings.Join(enabledList(cfg.Sections), ",")),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("proxy.shutdown")
		shCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(shCtx); err != nil {
			log.Error("proxy.shutdown_error", slog.String("error", err.Error()))
		}
		log.Info("proxy.stopped")
	case err := <-errCh:
		log.Error("proxy.serve_error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

type config struct {
	ListenAddr   string
	DockerSocket string
	LogLevel     slog.Level
	LogFormat    string
	LogPayload   bool
	Readonly     bool
	Sections     map[string]bool
}

func loadConfig() *config {
	c := &config{
		ListenAddr:   getenv("LISTEN_ADDR", defaultListen),
		DockerSocket: getenv("DOCKER_SOCKET", defaultSocket),
		LogLevel:     parseLevel(getenv("LOG_LEVEL", "info")),
		LogFormat:    parseFormat(getenv("LOG_FORMAT", "console")),
		LogPayload:   parseBool(os.Getenv("LOG_PAYLOAD"), false),
		Readonly:     parseBool(os.Getenv("READONLY"), true),
		Sections:     map[string]bool{},
	}
	for _, s := range proxy.AllSections() {
		c.Sections[s] = parseBool(os.Getenv(s), false)
	}
	return c
}

func getenv(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

func parseBool(v string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true":
		return true
	case "false":
		return false
	case "":
		return def
	default:
		return false
	}
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

// parseFormat maps LOG_FORMAT to a handler name; anything but "json" is console.
func parseFormat(s string) string {
	if strings.ToLower(strings.TrimSpace(s)) == "json" {
		return "json"
	}
	return "console"
}

func newLogger(lvl slog.Level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: lvl}
	if format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	// ponytail: stdlib TextHandler is the console format. No colors/alignment
	// until someone actually reads these by eye and complains.
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func logStartup(log *slog.Logger, c *config) {
	log.Info("proxy.starting",
		slog.Int("pid", os.Getpid()),
		slog.String("listen", c.ListenAddr),
		slog.String("socket", c.DockerSocket),
		slog.String("log_level", c.LogLevel.String()),
		slog.String("log_format", c.LogFormat),
		slog.Bool("readonly", c.Readonly),
		slog.Bool("log_payload", c.LogPayload),
	)
	if c.LogPayload {
		log.Warn("log_payload.enabled",
			slog.String("note", "request and response bodies will be logged (capped 8KiB); secrets may leak"),
		)
	}
	for _, s := range enabledList(c.Sections) {
		log.Info("section.enabled", slog.String("section", s))
	}
}

func enabledList(sections map[string]bool) []string {
	out := []string{}
	for k, v := range sections {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
