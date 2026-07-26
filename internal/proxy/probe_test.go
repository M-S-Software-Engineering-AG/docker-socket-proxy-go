package proxy

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProbe_Success(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "d.sock")

	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/_ping", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Api-Version", "1.54")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(l)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ver, err := Probe(ctx, sock, 2*time.Second, 100*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if ver != "1.54" {
		t.Errorf("api ver = %q, want 1.54", ver)
	}
}

func TestProbe_Timeout(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "missing.sock")
	// ensure absent
	_ = os.Remove(sock)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	_, err := Probe(ctx, sock, 600*time.Millisecond, 200*time.Millisecond, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Errorf("probe took %s, expected ~600ms", d)
	}
}
