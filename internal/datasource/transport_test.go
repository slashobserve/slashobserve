package datasource

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func burst(t *testing.T, client *http.Client, url string, n int) {
	t.Helper()
	release := make(chan struct{})
	var started, finished sync.WaitGroup
	started.Add(n)
	finished.Add(n)
	for range n {
		go func() {
			defer finished.Done()
			resp, err := client.Get(url)
			started.Done()
			if err != nil {
				t.Errorf("get: %v", err)
				return
			}
			<-release
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}()
	}
	started.Wait()
	close(release)
	finished.Wait()
}

func TestNewHTTPTransportKeepsConnectionsOfConcurrentCalls(t *testing.T) {
	var opened atomic.Int64
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()

	transport := NewHTTPTransport()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}

	const concurrent = 32
	burst(t, client, srv.URL, concurrent)
	before := opened.Load()
	burst(t, client, srv.URL, concurrent)
	if redialed := opened.Load() - before; redialed != 0 {
		t.Fatalf("the second burst of %d concurrent calls opened %d new connections, want 0", concurrent, redialed)
	}
}
