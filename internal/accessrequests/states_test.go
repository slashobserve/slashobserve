package accessrequests

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

func TestStateOfTellsStillWaitingFromNotGranted(t *testing.T) {
	cases := []struct {
		name string
		in   standing
		want State
	}{
		{"just asked", standing{}, StateOpen},
		{"on the last open day", standing{age: OpenFor}, StateOpen},
		{"past the open window", standing{age: OpenFor + time.Second}, StateExpired},
		{"marked done by a granter", standing{dismissed: true}, StateDismissed},
		{"dismissed long ago", standing{dismissed: true, age: 2 * OpenFor}, StateDismissed},
		{"asked again after a dismissal was undone", standing{superseded: true}, StateDismissed},
		{"asked again and expired since", standing{superseded: true, age: OpenFor + time.Second}, StateExpired},
		{"granted", standing{granted: true}, StateGranted},
		{"granted long ago", standing{granted: true, age: 2 * OpenFor}, StateGranted},
	}
	for _, c := range cases {
		if got := stateOf(c.in); got != c.want {
			t.Errorf("%s: state = %q, want %q", c.name, got, c.want)
		}
		if !States.Has(string(stateOf(c.in))) {
			t.Errorf("%s: %q is not a declared state", c.name, stateOf(c.in))
		}
	}
}

func TestEveryHTTPAnswerCarriesTheState(t *testing.T) {
	raw, err := json.Marshal(statedListJSONOf([]Request{{ID: "r1", State: StateDismissed}}))
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Items []struct {
			ID    string `json:"id"`
			State State  `json:"state"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &list); err != nil || len(list.Items) != 1 || list.Items[0].ID != "r1" || list.Items[0].State != StateDismissed {
		t.Fatalf("list = %s (%v), want r1 dismissed", raw, err)
	}
}

func serveMine(t *testing.T, r rig, by auth.Principal) []statedJSON {
	t.Helper()
	engine := gin.New()
	api := engine.Group("/api/v1", func(c *gin.Context) {
		c.Request = c.Request.WithContext(auth.WithPrincipal(c.Request.Context(), by))
	})
	NewHandler(r.service, slog.New(slog.DiscardHandler)).Register(auth.NewRoutes(api))
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/access-requests?scope=mine", nil))
	var list statedListJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("%s's own list = %d %s", by.Name, rec.Code, rec.Body)
	}
	return list.Items
}

func statesOf(items []statedJSON) []State {
	out := make([]State, 0, len(items))
	for _, item := range items {
		out = append(out, item.State)
	}
	return out
}

func TestTheRequesterReadsWhereEachOfTheirRequestsStands(t *testing.T) {
	r := newRig(t)
	folder := r.folderBy(t, "Payments", ben)
	waiting := r.ask(t, ada, Object{Kind: KindDashboard, ID: r.dashboard(t, "Waiting", folder)}, "")
	dismissed := r.ask(t, ada, Object{Kind: KindDashboard, ID: r.dashboard(t, "Dismissed", folder)}, "")
	expired := r.ask(t, ada, Object{Kind: KindDashboard, ID: r.dashboard(t, "Expired", folder)}, "")
	r.ask(t, cleo, Object{Kind: KindDashboard, ID: r.dashboard(t, "Someone else's", folder)}, "")

	if got := statesOf(serveMine(t, r, ada)); !slices.Equal(got, []State{StateOpen, StateOpen, StateOpen}) {
		t.Fatalf("Ada's requests = %v, want three open ones and not Cleo's", got)
	}
	for _, item := range r.inbox(t, ben, activity.TriageOpen) {
		if item.TargetID == dismissed.ID {
			if err := r.store.SetItemDone(t.Context(), ben, item.Seq, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	r.db.Exec(t, `UPDATE access_requests SET created_at = now() - interval '31 days' WHERE id = $1::uuid`, expired.ID)

	want := map[string]State{waiting.ID: StateOpen, dismissed.ID: StateDismissed, expired.ID: StateExpired}
	for _, item := range serveMine(t, r, ada) {
		if item.State != want[item.ID] {
			t.Errorf("request on %q is %q, want %q", item.Object.Title, item.State, want[item.ID])
		}
	}

	r.grant(t, folder, ada, "comment")
	if err := r.db.InTx(t.Context(), func(q *dbsqlc.Queries) error {
		return r.service.SettleFolder(t.Context(), q, ben, folder)
	}); err != nil {
		t.Fatal(err)
	}
	for _, item := range serveMine(t, r, ada) {
		if item.ID == waiting.ID && item.State != StateGranted {
			t.Errorf("the open request after the grant is %q, want granted", item.State)
		}
		if item.ID == dismissed.ID && item.State != StateDismissed {
			t.Errorf("the dismissed request after the grant is %q, want it to stay dismissed", item.State)
		}
	}

	rec := serve(t, r, ada, http.MethodGet, "")
	if rec.Code != http.StatusOK || rec.Body.String() != `{"items":[]}` {
		t.Fatalf("the default scope for Ada = %d %s, want the empty managed list", rec.Code, rec.Body)
	}
}
