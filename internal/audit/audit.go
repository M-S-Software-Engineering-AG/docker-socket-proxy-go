package audit

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
)

// MaxBody is the per-direction cap on captured payload bytes.
const MaxBody = 8 * 1024

// SkipBody returns true when a body should not be captured: hijacked
// connections (attach/exec), tar streams, raw streams, or arbitrary
// binary streams that may run for the lifetime of the request.
func SkipBody(h http.Header) bool {
	if h.Get("Upgrade") != "" {
		return true
	}
	ct := strings.ToLower(h.Get("Content-Type"))
	switch {
	case strings.HasPrefix(ct, "application/vnd.docker.raw-stream"),
		strings.HasPrefix(ct, "application/vnd.docker.multiplexed-stream"),
		strings.HasPrefix(ct, "application/x-tar"),
		strings.HasPrefix(ct, "application/octet-stream"):
		return true
	}
	return false
}

// CapturedReader wraps an io.ReadCloser, copying up to MaxBody bytes into
// Buf for later inspection while still yielding the original stream.
type CapturedReader struct {
	src io.ReadCloser
	Buf bytes.Buffer
}

func NewCapturedReader(src io.ReadCloser) *CapturedReader {
	return &CapturedReader{src: src}
}

func (c *CapturedReader) Read(p []byte) (int, error) {
	n, err := c.src.Read(p)
	if n > 0 && c.Buf.Len() < MaxBody {
		room := MaxBody - c.Buf.Len()
		if n < room {
			room = n
		}
		c.Buf.Write(p[:room])
	}
	return n, err
}

func (c *CapturedReader) Close() error { return c.src.Close() }

// CaptureWriter wraps an http.ResponseWriter and tees the first MaxBody
// bytes written into Buf. Implements http.Flusher / Hijacker passthrough
// so streaming + upgrade still work.
type CaptureWriter struct {
	http.ResponseWriter
	Buf       bytes.Buffer
	Status    int
	wroteHead bool
}

func (w *CaptureWriter) WriteHeader(code int) {
	w.Status = code
	w.wroteHead = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *CaptureWriter) Write(p []byte) (int, error) {
	if !w.wroteHead {
		w.Status = http.StatusOK
		w.wroteHead = true
	}
	if w.Buf.Len() < MaxBody {
		room := MaxBody - w.Buf.Len()
		if len(p) < room {
			room = len(p)
		}
		w.Buf.Write(p[:room])
	}
	return w.ResponseWriter.Write(p)
}

func (w *CaptureWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack passes through to the underlying writer so HTTP upgrade flows
// (attach, exec start) keep working when audit middleware wraps the
// response writer.
func (w *CaptureWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("audit: underlying ResponseWriter does not implement http.Hijacker")
}
