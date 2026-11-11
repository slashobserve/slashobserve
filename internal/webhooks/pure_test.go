package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/slashobserve/slashobserve/internal/activity"
	"github.com/slashobserve/slashobserve/internal/egress"
)

func TestNextAttemptBacksOffAndCaps(t *testing.T) {
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{1, 5 * time.Second}, {2, 10 * time.Second}, {3, 20 * time.Second}, {4, 40 * time.Second},
		{5, 80 * time.Second}, {6, 160 * time.Second}, {7, 320 * time.Second}, {8, 640 * time.Second},
		{9, 15 * time.Minute}, {10, 15 * time.Minute}, {50, 15 * time.Minute}, {1 << 30, 15 * time.Minute},
	}
	for _, tc := range cases {
		if got := nextAttempt(tc.failures, 500, 0); got != tc.want {
			t.Errorf("failure %d: %v, want %v", tc.failures, got, tc.want)
		}
	}
}

func TestNextAttemptTakesRetryAfterOnlyOn429And503AndClampsIt(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		retryAfter time.Duration
		want       time.Duration
	}{
		{"429 asks for a minute", 429, time.Minute, time.Minute},
		{"503 asks for a minute", 503, time.Minute, time.Minute},
		{"below the floor", 429, time.Second, 5 * time.Second},
		{"above the ceiling", 503, 5 * time.Hour, time.Hour},
		{"500 ignores it", 500, time.Minute, 10 * time.Second},
		{"a failure with no status ignores it", 0, time.Minute, 10 * time.Second},
		{"429 without the header backs off", 429, 0, 10 * time.Second},
	}
	for _, tc := range cases {
		if got := nextAttempt(2, tc.status, tc.retryAfter); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
}

type scripted struct {
	result egress.SendResult
	err    error
}

func (s scripted) Send(context.Context, egress.SendRequest) (egress.SendResult, error) {
	return s.result, s.err
}

func TestDeliverClassifiesTheAnswer(t *testing.T) {
	host := func(err error) error { return &egress.HostError{Host: receiverHost, Err: err} }
	cases := []struct {
		name    string
		sender  scripted
		outcome string
		code    string
		status  int
		gone    bool
	}{
		{"200", scripted{result: egress.SendResult{Status: 200}}, outcomeDelivered, "", 200, false},
		{"204", scripted{result: egress.SendResult{Status: 204}}, outcomeDelivered, "", 204, false},
		{"299", scripted{result: egress.SendResult{Status: 299}}, outcomeDelivered, "", 299, false},
		{"301", scripted{result: egress.SendResult{Status: 301}}, outcomeFailed, errorStatus, 301, false},
		{"400", scripted{result: egress.SendResult{Status: 400}}, outcomeFailed, errorStatus, 400, false},
		{"410", scripted{result: egress.SendResult{Status: 410}}, outcomeFailed, errorStatus, 410, true},
		{"429", scripted{result: egress.SendResult{Status: 429, RetryAfter: time.Minute}}, outcomeFailed, errorStatus, 429, false},
		{"500", scripted{result: egress.SendResult{Status: 500}}, outcomeFailed, errorStatus, 500, false},
		{"too large", scripted{err: host(egress.ErrTooLarge)}, outcomeSkipped, errorTooLarge, 0, false},
		{"blocked address", scripted{err: host(egress.ErrBlockedAddress)}, outcomeFailed, errorBlockedAddress, 0, false},
		{"domain not allowed", scripted{err: host(egress.ErrNotAllowed)}, outcomeFailed, errorDomainNotAllowed, 0, false},
		{"unreachable", scripted{err: host(egress.ErrUnavailable)}, outcomeFailed, errorUnreachable, 0, false},
		{"anything else", scripted{err: errors.New("boom")}, outcomeFailed, errorUnreachable, 0, false},
	}
	for _, tc := range cases {
		got, err := deliver(t.Context(), tc.sender, time.Unix(1700000000, 0), receiverURL, "whsec_test", "id.1", "finding.created", []byte("{}"))
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.outcome != tc.outcome || got.errorCode != tc.code || got.status != tc.status || got.gone != tc.gone {
			t.Errorf("%s: %+v, want outcome %s code %s status %d gone %v", tc.name, got, tc.outcome, tc.code, tc.status, tc.gone)
		}
	}
}

func TestDeliverReturnsACancellationInsteadOfAnAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := deliver(ctx, scripted{err: &egress.HostError{Host: receiverHost, Err: egress.ErrUnavailable}}, time.Now(), receiverURL, "whsec_test", "id.1", "finding.created", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v, want context.Canceled", err)
	}
}

type capture struct{ got egress.SendRequest }

func (c *capture) Send(_ context.Context, req egress.SendRequest) (egress.SendResult, error) {
	c.got = req
	return egress.SendResult{Status: 204}, nil
}

func TestDeliverSendsTheHeadersAndAFreshSignature(t *testing.T) {
	c := &capture{}
	at := time.Unix(1700000000, 0)
	if _, err := deliver(t.Context(), c, at, receiverURL, "whsec_test", "wh.7", "finding.created", []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Content-Type":             "application/json",
		"X-Slashobserve-Event":     "finding.created",
		"X-Slashobserve-Delivery":  "wh.7",
		"X-Slashobserve-Signature": "t=1700000000,v1=85876387ad9d6be57a04653bc0729da757049f58afb10ba6cac3bedaecf4fda3",
	}
	for name, value := range want {
		if got := c.got.Header.Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
	if c.got.Method != http.MethodPost || c.got.URL != receiverURL || string(c.got.Body) != `{"ok":true}` {
		t.Errorf("request %+v", c.got)
	}
}

func TestSignatureMatchesTheDocumentedVector(t *testing.T) {
	got := signature("whsec_test", time.Unix(1700000000, 0), []byte(`{"ok":true}`))
	want := "t=1700000000,v1=85876387ad9d6be57a04653bc0729da757049f58afb10ba6cac3bedaecf4fda3"
	if got != want {
		t.Fatalf("signature %q, want %q", got, want)
	}
}

func TestEnvelopeKeysAreExactlyTheDocumentedOnes(t *testing.T) {
	e := activity.Event{Seq: 7, Type: activity.TypeFindingCreated, TargetKind: "finding", TargetID: "t", Data: json.RawMessage(`{"title":"x"}`)}
	body, deliveryID, err := eventPayload("wh-1", e)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	if want := []string{"delivery_id", "event", "type", "webhook_id"}; !slices.Equal(keys, want) {
		t.Fatalf("keys %v, want %v", keys, want)
	}
	if deliveryID != "wh-1.7" || string(got["delivery_id"]) != `"wh-1.7"` || string(got["type"]) != `"finding.created"` {
		t.Fatalf("envelope %s", body)
	}
	var event activity.EventJSON
	if err := json.Unmarshal(got["event"], &event); err != nil || event.Seq != 7 || string(event.Data) != `{"title":"x"}` {
		t.Fatalf("event %s, %v", got["event"], err)
	}
}

func TestTestEnvelopeHasNoEvent(t *testing.T) {
	body, deliveryID, err := testPayload("wh-1")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		DeliveryID string          `json:"delivery_id"`
		WebhookID  string          `json:"webhook_id"`
		Type       string          `json:"type"`
		Event      json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != "webhook.test" || string(got.Event) != "null" || got.WebhookID != "wh-1" || got.DeliveryID != deliveryID {
		t.Fatalf("envelope %s", body)
	}
	prefix := "wh-1.test."
	if !strings.HasPrefix(deliveryID, prefix) || len(deliveryID) != len(prefix)+12 {
		t.Fatalf("delivery id %q", deliveryID)
	}
}

func TestNewSecretHasThePrefixAndFortyThreeCharacters(t *testing.T) {
	a, err := newSecret()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := newSecret()
	if !strings.HasPrefix(a, "whsec_") || len(a) != len("whsec_")+43 || a == b {
		t.Fatalf("secrets %q and %q", a, b)
	}
}

func TestCleanNameAndURLAndEventTypes(t *testing.T) {
	for _, name := range []string{"", "   ", strings.Repeat("n", 101), "a\x00b", "line\nbreak"} {
		if _, err := cleanName(name); err == nil {
			t.Errorf("name %q accepted", name)
		}
	}
	if got, err := cleanName("  Pipeline  "); err != nil || got != "Pipeline" {
		t.Errorf("name: %q, %v", got, err)
	}
	if _, err := cleanName(strings.Repeat("é", 100)); err != nil {
		t.Errorf("100 runes of two bytes refused: %v", err)
	}

	bad := []string{
		"", "http://hooks.example.test/in", "https://user@hooks.example.test/in", "https://hooks.example.test:8443/in",
		"https://hooks.example.test/in#frag", "https://10.0.0.1/in", "https://localhost/in", "ftp://hooks.example.test",
		"hooks.example.test/in", "https://" + strings.Repeat("a", 2050) + ".test/",
	}
	for _, raw := range bad {
		if _, _, err := cleanURL(raw); err == nil {
			t.Errorf("url %q accepted", raw)
		}
	}
	for raw, want := range map[string]string{
		"https://HOOKS.example.test/in?room=abc": "https://hooks.example.test/in?room=abc",
		"https://hooks.example.test:443/in":      "https://hooks.example.test/in",
		"  https://hooks.example.test/a/b  ":     "https://hooks.example.test/a/b",
	} {
		got, host, err := cleanURL(raw)
		if err != nil || got != want || host != "hooks.example.test" {
			t.Errorf("url %q: %q, %q, %v; want %q", raw, got, host, err, want)
		}
	}

	all := activity.Types()
	if _, err := cleanEventTypes(all); err != nil {
		t.Errorf("every type refused: %v", err)
	}
	for name, types := range map[string][]string{
		"none":      nil,
		"unknown":   {"dashboard.exploded"},
		"duplicate": {activity.TypeFindingCreated, activity.TypeFindingCreated},
		"too many":  append(slices.Clone(all), activity.TypeRunQueued),
	} {
		if _, err := cleanEventTypes(types); err == nil {
			t.Errorf("event types %s accepted", name)
		}
	}
}
