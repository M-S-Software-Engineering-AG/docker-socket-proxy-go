package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"dockersocketproxy/internal/audit"
)

// Handler builds the request flow: classify -> deny or proxy -> audit.
type Handler struct {
	Cfg         *Config
	Socket      string
	Log         *slog.Logger
	LogPayload  bool

	rp *httputil.ReverseProxy
}

// NewHandler wires a ReverseProxy with a unix-socket transport.
func NewHandler(cfg *Config, socket string, log *slog.Logger, logPayload bool) *Handler {
	target, _ := url.Parse("http://docker")
	rp := httputil.NewSingleHostReverseProxy(target)
	rp.Transport = &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, "unix", socket)
		},
		MaxIdleConns:          16,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 0, // streaming endpoints can take arbitrarily long
		ExpectContinueTimeout: 1 * time.Second,
	}
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Error("proxy.upstream_error",
			slog.String("path", r.URL.Path),
			slog.String("error", err.Error()),
		)
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
	}
	return &Handler{Cfg: cfg, Socket: socket, Log: log, LogPayload: logPayload, rp: rp}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	res := h.Cfg.Decide(r.Method, r.URL.Path)

	if res.Decision != Allow {
		http.Error(w, denyMessage(res), res.Status)
		h.audit(r, res, res.Status, start, nil, nil)
		return
	}

	// Optional payload capture (skipped for streams).
	var reqBody *audit.CapturedReader
	if h.LogPayload && r.Body != nil && !audit.SkipBody(r.Header) {
		reqBody = audit.NewCapturedReader(r.Body)
		r.Body = reqBody
	}

	if h.LogPayload {
		cw := &audit.CaptureWriter{ResponseWriter: w}
		h.rp.ServeHTTP(cw, r)
		h.audit(r, res, cw.Status, start, reqBody, &cw.Buf)
		return
	}

	sw := &statusWriter{ResponseWriter: w}
	h.rp.ServeHTTP(sw, r)
	h.audit(r, res, sw.status, start, nil, nil)
}

func denyMessage(r Result) string {
	switch r.Decision {
	case DenyDisabled:
		return fmt.Sprintf("section %s disabled", r.Section)
	case DenyReadonly:
		return "proxy is in readonly mode; only GET/HEAD allowed"
	case DenyUnknown:
		return "unknown docker api path"
	}
	return "denied"
}

func (h *Handler) audit(r *http.Request, res Result, status int, start time.Time,
	reqBody *audit.CapturedReader, respBody interface{ String() string }) {

	attrs := []slog.Attr{
		slog.String("remote", r.RemoteAddr),
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.String("section", res.Section),
		slog.String("decision", string(res.Decision)),
		slog.Int("status", status),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if reqBody != nil && reqBody.Buf.Len() > 0 {
		attrs = append(attrs, slog.String("req_body", reqBody.Buf.String()))
	}
	if respBody != nil && respBody.String() != "" {
		attrs = append(attrs, slog.String("resp_body", respBody.String()))
	}
	h.Log.LogAttrs(r.Context(), slog.LevelInfo, "audit", attrs...)
}

// statusWriter captures the response status when payload capture is off
// while preserving Hijacker / Flusher passthrough.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("proxy: underlying ResponseWriter does not implement http.Hijacker")
}
