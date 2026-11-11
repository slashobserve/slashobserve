package mcpauth

import (
	"net/url"
	"testing"
	"time"
)

func TestLimiterRefusesPastTheLimitAndResetsWithTheWindow(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	l := newLimiter(registerPerMinute, time.Minute, nil)
	l.now = func() time.Time { return now }
	for i := 1; i <= registerPerMinute; i++ {
		if !l.allow("") {
			t.Fatalf("request %d refused", i)
		}
	}
	if l.allow("") {
		t.Fatal("the 31st request in a minute was allowed")
	}
	if got := l.retryAfterSeconds(); got != 60 {
		t.Fatalf("retry after %d, want 60", got)
	}
	now = now.Add(20 * time.Second)
	if got := l.retryAfterSeconds(); got != 40 {
		t.Fatalf("retry after %d, want 40", got)
	}
	now = now.Add(40 * time.Second)
	if !l.allow("") {
		t.Fatal("the first request of the next minute was refused")
	}
}

func TestLimiterKeysAreIndependent(t *testing.T) {
	l := newLimiter(1, time.Minute, nil)
	if !l.allow("a") || l.allow("a") {
		t.Fatal("key a should be allowed once")
	}
	if !l.allow("b") {
		t.Fatal("key b was refused because of key a")
	}
}

func halve(total int) int { return (total + 1) / 2 }

func TestLimiterAllowsOnlyItsShareOfTheTotal(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	l := newLimiter(registerPerMinute, time.Minute, halve)
	l.now = func() time.Time { return now }
	for i := 1; i <= registerPerMinute/2; i++ {
		if !l.allow("") {
			t.Fatalf("request %d refused", i)
		}
	}
	if l.allow("") {
		t.Fatalf("request %d was allowed with half the total", registerPerMinute/2+1)
	}
	now = now.Add(time.Minute)
	if !l.allow("") {
		t.Fatal("the first request of the next minute was refused")
	}
}

func TestShareAppliesToTheOverallLimitsAndNotToTheClientLimit(t *testing.T) {
	s := New(Config{PublicURL: &url.URL{Scheme: "http", Host: "observe.test:8080"}, Share: halve}, nil, nil)
	for name, l := range map[string]*limiter{"register": s.registerLimit, "authorize": s.authorizeLimit, "token": s.tokenLimit, "revoke": s.revokeLimit} {
		if got, want := l.share(l.limit), (l.limit+1)/2; got != want {
			t.Errorf("%s allows %d of %d, want %d", name, got, l.limit, want)
		}
	}
	for i := 1; i <= tokenPerClientPerMinute; i++ {
		if !s.tokenClient.allow("client") {
			t.Fatalf("per-client request %d refused", i)
		}
	}
	if s.tokenClient.allow("client") {
		t.Fatalf("per-client request %d was allowed", tokenPerClientPerMinute+1)
	}
}
