package telemetry

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeHijackWriter struct {
	*httptest.ResponseRecorder
	hijacked bool
	flushed  bool
}

func (f *fakeHijackWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	f.hijacked = true
	return nil, nil, nil
}

func (f *fakeHijackWriter) Flush() {
	f.flushed = true
}

func TestRequestLoggerPreservesHijackerAndFlusher(t *testing.T) {
	var sawHijacker bool
	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, ok := w.(http.Hijacker); ok {
			_, _, _ = h.Hijack()
			sawHijacker = true
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		rc := http.NewResponseController(w)
		if _, _, err := rc.Hijack(); err != nil {
			t.Fatalf("ResponseController.Hijack failed: %v", err)
		}
	}))

	fw := &fakeHijackWriter{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/vnc/stream", nil)
	handler.ServeHTTP(fw, req)

	if !sawHijacker || !fw.hijacked {
		t.Fatal("expected RequestLogger ResponseWriter to preserve http.Hijacker")
	}
	if !fw.flushed {
		t.Fatal("expected RequestLogger ResponseWriter to preserve http.Flusher")
	}
}
