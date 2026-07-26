package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Probe verifies the docker socket is reachable by dialling it and issuing
// GET /_ping. It retries every interval until ok or timeout elapses.
//
// Returns the engine API version reported via the Api-Version header on
// success, or an error after timeout.
func Probe(ctx context.Context, socketPath string, timeout, interval time.Duration, log *slog.Logger) (string, error) {
	deadline := time.Now().Add(timeout)
	attempt := 0
	var lastErr error

	for {
		attempt++
		apiVer, err := pingOnce(ctx, socketPath)
		if err == nil {
			return apiVer, nil
		}
		lastErr = err

		if log != nil {
			log.Warn("socket.retry",
				slog.Int("attempt", attempt),
				slog.String("socket", socketPath),
				slog.String("error", err.Error()),
			)
		}

		if time.Now().Add(interval).After(deadline) {
			return "", fmt.Errorf("socket %s unreachable after %s (%d attempts): %w",
				socketPath, timeout, attempt, lastErr)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(interval):
		}
	}
}

func pingOnce(ctx context.Context, socketPath string) (string, error) {
	tr := &http.Transport{
		DialContext: func(dctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 1 * time.Second}
			return d.DialContext(dctx, "unix", socketPath)
		},
		DisableKeepAlives: true,
	}
	client := &http.Client{Transport: tr, Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/_ping", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ping: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ping status %d", resp.StatusCode)
	}
	return resp.Header.Get("Api-Version"), nil
}
