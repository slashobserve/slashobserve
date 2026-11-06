package server

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func compressedEngine(t *testing.T, register func(*gin.Engine)) http.Handler {
	t.Helper()
	engine := gin.New()
	register(engine)
	handler, err := compressReads(engine)
	if err != nil {
		t.Fatalf("compressReads: %v", err)
	}
	return handler
}

func repetitiveJSON(size int) []byte {
	var b bytes.Buffer
	b.WriteString(`{"values":[`)
	for i := 0; b.Len() < size; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`[1762387200,"0.25"]`)
	}
	b.WriteString(`]}`)
	return b.Bytes()
}

func TestCompressReadsByMethodSizeAndAcceptEncoding(t *testing.T) {
	large := repetitiveJSON(300 << 10)
	small := repetitiveJSON(512)
	handler := compressedEngine(t, func(e *gin.Engine) {
		serve := func(body []byte) gin.HandlerFunc {
			return func(c *gin.Context) { c.Data(http.StatusOK, "application/json", body) }
		}
		e.GET("/large", serve(large))
		e.POST("/large", serve(large))
		e.GET("/small", serve(small))
	})
	cases := []struct {
		name, method, path string
		acceptGzip, want   bool
		body               []byte
	}{
		{"large get with gzip", http.MethodGet, "/large", true, true, large},
		{"large get without accept-encoding", http.MethodGet, "/large", false, false, large},
		{"get under the size floor", http.MethodGet, "/small", true, false, small},
		{"large post", http.MethodPost, "/large", true, false, large},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.acceptGzip {
				req.Header.Set("Accept-Encoding", "gzip")
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			encoding := rec.Header().Get("Content-Encoding")
			if !tc.want {
				if encoding != "" {
					t.Fatalf("Content-Encoding = %q, want none", encoding)
				}
				if !bytes.Equal(rec.Body.Bytes(), tc.body) {
					t.Fatalf("plain body differs: got %d bytes, want %d", rec.Body.Len(), len(tc.body))
				}
				return
			}
			if encoding != "gzip" {
				t.Fatalf("Content-Encoding = %q, want gzip", encoding)
			}
			if vary := rec.Header().Values("Vary"); !containsFold(vary, "Accept-Encoding") {
				t.Fatalf("Vary = %q, want Accept-Encoding", vary)
			}
			if rec.Body.Len()*5 >= len(tc.body) {
				t.Fatalf("compressed %d bytes to %d, want under a fifth", len(tc.body), rec.Body.Len())
			}
			zr, err := gzip.NewReader(rec.Body)
			if err != nil {
				t.Fatalf("gzip reader: %v", err)
			}
			plain, err := io.ReadAll(zr)
			if err != nil {
				t.Fatalf("gunzip: %v", err)
			}
			if !bytes.Equal(plain, tc.body) {
				t.Fatalf("gunzipped body differs: got %d bytes, want %d", len(plain), len(tc.body))
			}
		})
	}
}

func containsFold(values []string, want string) bool {
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), want) {
				return true
			}
		}
	}
	return false
}

type firstEvent struct {
	encoding, event string
	err             error
}

func readFirstEvent(url string, size int) firstEvent {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return firstEvent{err: err}
	}
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return firstEvent{err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	buf := make([]byte, size)
	if _, err := io.ReadFull(bufio.NewReader(resp.Body), buf); err != nil {
		return firstEvent{err: err}
	}
	return firstEvent{encoding: resp.Header.Get("Content-Encoding"), event: string(buf)}
}

func TestCompressReadsStreamsEventsUncompressed(t *testing.T) {
	const event = "event: ready\ndata: {}\n\n"
	release := make(chan struct{})
	handler := compressedEngine(t, func(e *gin.Engine) {
		e.GET("/events", func(c *gin.Context) {
			c.Header("Content-Type", "text/event-stream")
			c.Status(http.StatusOK)
			_, _ = c.Writer.WriteString(event)
			c.Writer.Flush()
			select {
			case <-release:
			case <-c.Request.Context().Done():
			}
		})
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	got := make(chan firstEvent, 1)
	go func() { got <- readFirstEvent(srv.URL+"/events", len(event)) }()
	select {
	case first := <-got:
		if first.err != nil {
			t.Fatalf("read first event: %v", first.err)
		}
		if first.encoding != "" {
			t.Fatalf("Content-Encoding = %q, want none", first.encoding)
		}
		if first.event != event {
			t.Fatalf("first event = %q, want %q", first.event, event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first event not received before release: the stream is buffered")
	}
}

func TestCompressReadsKeepsResponseControllerFlush(t *testing.T) {
	flushed := make(chan error, 1)
	body := repetitiveJSON(4 << 10)
	handler := compressedEngine(t, func(e *gin.Engine) {
		e.GET("/flush", gin.WrapH(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
			flushed <- http.NewResponseController(w).Flush()
		})))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/flush", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get /flush: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := <-flushed; err != nil {
		t.Fatalf("ResponseController.Flush: %v", err)
	}
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", encoding)
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	if !bytes.Equal(plain, body) {
		t.Fatalf("gunzipped body differs: got %d bytes, want %d", len(plain), len(body))
	}
}
