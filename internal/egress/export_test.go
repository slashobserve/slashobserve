package egress

import (
	"net/http"
	"testing"
)

func NewTestClient(t *testing.T, allow Allowlist, mux *http.ServeMux) (*Client, func() int64) {
	t.Helper()
	up, c := newUpstream(t, allow, mux)
	return c, up.hits.Load
}
