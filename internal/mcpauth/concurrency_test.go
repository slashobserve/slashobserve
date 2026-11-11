package mcpauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func concurrently(n int, call func(i int) *httptest.ResponseRecorder) []*httptest.ResponseRecorder {
	out := make([]*httptest.ResponseRecorder, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			out[i] = call(i)
		}()
	}
	close(start)
	wg.Wait()
	return out
}

func statuses(recs []*httptest.ResponseRecorder) map[int]int {
	counts := map[int]int{}
	for _, rec := range recs {
		counts[rec.Code]++
	}
	return counts
}

func assertNo5xx(t *testing.T, recs []*httptest.ResponseRecorder) {
	t.Helper()
	for _, rec := range recs {
		if rec.Code >= 500 {
			t.Errorf("status %d: %s", rec.Code, rec.Body.String())
		}
	}
}

func (r *rig) liveGrants(t *testing.T, userID, clientID string) int {
	t.Helper()
	var n int
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM oauth_grants WHERE user_id = $1 AND client_id = $2 AND revoked_at IS NULL`, userID, clientID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *rig) liveGrantsOf(t *testing.T, userID string) int {
	t.Helper()
	var n int
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM oauth_grants WHERE user_id = $1 AND revoked_at IS NULL`, userID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func decodeTokens(t *testing.T, rec *httptest.ResponseRecorder) tokens {
	t.Helper()
	var out tokens
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("token body %q: %v", rec.Body.String(), err)
	}
	return out
}

func TestTwoExchangesOfOneCodeAtOnceIssueOnceAndRevokeTheGrant(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	code := r.code(t, ada, clientID, callback)

	recs := concurrently(2, func(int) *httptest.ResponseRecorder {
		return r.oauth(http.MethodPost, "/oauth/token", exchangeForm(clientID, code))
	})

	assertNo5xx(t, recs)
	if got := statuses(recs); got[http.StatusOK] != 1 || got[http.StatusBadRequest] != 1 {
		t.Fatalf("statuses %v, want one 200 and one 400", got)
	}
	for _, rec := range recs {
		if rec.Code == http.StatusOK {
			if _, _, ok := r.authenticate(t, decodeTokens(t, rec).Access); ok {
				t.Error("the access token works after its code was used twice")
			}
		}
	}
	if reason := r.revokedReason(t, ada.ID, clientID); reason != "code_reuse" {
		t.Fatalf("revoked reason %q, want code_reuse", reason)
	}
}

func TestTwoCodesForOneClientAndPersonAtOnceLeaveOneLiveGrant(t *testing.T) {
	r := newRig(t)
	clientID := r.registerClient(t, "claude-code")
	codes := []string{r.code(t, ada, clientID, callback), r.code(t, ada, clientID, callback)}

	recs := concurrently(2, func(i int) *httptest.ResponseRecorder {
		return r.oauth(http.MethodPost, "/oauth/token", exchangeForm(clientID, codes[i]))
	})

	assertNo5xx(t, recs)
	if got := statuses(recs); got[http.StatusOK] != 2 {
		t.Fatalf("statuses %v, want two 200s", got)
	}
	if n := r.liveGrants(t, ada.ID, clientID); n != 1 {
		t.Fatalf("%d live grants, want 1", n)
	}
	working := 0
	for _, rec := range recs {
		if _, _, ok := r.authenticate(t, decodeTokens(t, rec).Access); ok {
			working++
		}
	}
	if working != 1 {
		t.Fatalf("%d access tokens work, want only the newer grant's", working)
	}
	var replaced int
	err := r.db.SQLDB(t).QueryRowContext(t.Context(), `SELECT count(*) FROM oauth_grants WHERE user_id = $1 AND revoked_reason = 'replaced'`, ada.ID).Scan(&replaced)
	if err != nil || replaced != 1 {
		t.Fatalf("%d replaced grants, err %v", replaced, err)
	}
}

func (r *rig) seedGrants(t *testing.T, userID string, n int) {
	t.Helper()
	r.db.Exec(t, `WITH c AS (INSERT INTO oauth_clients (id, redirect_uris) SELECT 'bulk-' || i, ARRAY['http://localhost/cb'] FROM generate_series(1, $2::int) i RETURNING id)
INSERT INTO oauth_grants (client_id, user_id, resource, expires_at) SELECT id, $1, 'r', now() + interval '1 day' FROM c`, userID, n)
}

func TestApprovalsAtOnceNeverPassTheCap(t *testing.T) {
	r := newRig(t)
	r.seedGrants(t, ada.ID, maxLiveGrantsPerPerson-1)
	first, second := r.registerClient(t, "first"), r.registerClient(t, "second")
	requests := []string{r.pendingRequest(t, first, callback), r.pendingRequest(t, second, callback)}

	recs := concurrently(2, func(i int) *httptest.ResponseRecorder {
		return r.api(ada, http.MethodPost, "/oauth/requests/"+requests[i]+"/approve")
	})

	assertNo5xx(t, recs)
	if got := statuses(recs); got[http.StatusOK] != 1 || got[http.StatusConflict] != 1 {
		t.Fatalf("statuses %v, want one 200 and one 409", got)
	}
	for i, rec := range recs {
		if rec.Code != http.StatusOK {
			continue
		}
		var out struct {
			RedirectTo string `json:"redirect_to"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		clientID := []string{first, second}[i]
		r.tokens(t, exchangeForm(clientID, redirectQuery(t, out.RedirectTo).Get("code")))
	}
	if n := r.liveGrantsOf(t, ada.ID); n != maxLiveGrantsPerPerson {
		t.Fatalf("%d live grants, want %d", n, maxLiveGrantsPerPerson)
	}
}

func TestAnApprovedButNotYetExchangedCodeCountsTowardTheCap(t *testing.T) {
	r := newRig(t)
	r.seedGrants(t, ada.ID, maxLiveGrantsPerPerson-1)
	first, second := r.registerClient(t, "first"), r.registerClient(t, "second")
	r.approve(t, ada, r.pendingRequest(t, first, callback))

	refused := r.api(ada, http.MethodPost, "/oauth/requests/"+r.pendingRequest(t, second, callback)+"/approve")
	if refused.Code != http.StatusConflict {
		t.Fatalf("approval beside a pending code = %d: %s", refused.Code, refused.Body.String())
	}
	again := r.api(ada, http.MethodPost, "/oauth/requests/"+r.pendingRequest(t, first, callback)+"/approve")
	if again.Code != http.StatusOK {
		t.Fatalf("approving the client that holds the pending code again = %d: %s", again.Code, again.Body.String())
	}
}

func TestApprovalsOfOtherPeopleAtOnceAreIndependent(t *testing.T) {
	r := newRig(t)
	r.seedGrants(t, ada.ID, maxLiveGrantsPerPerson-1)
	clientID := r.registerClient(t, "claude-code")
	people := []auth.Principal{ada, bob}
	requests := []string{r.pendingRequest(t, clientID, callback), r.pendingRequest(t, clientID, callback)}

	recs := concurrently(2, func(i int) *httptest.ResponseRecorder {
		return r.api(people[i], http.MethodPost, "/oauth/requests/"+requests[i]+"/approve")
	})

	assertNo5xx(t, recs)
	if got := statuses(recs); got[http.StatusOK] != 2 {
		t.Fatalf("statuses %v, want two 200s", got)
	}
}
