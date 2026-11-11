package mcpauth

import (
	"testing"
	"time"
)

func TestLimiterRefusesPastTheLimitAndResetsWithTheWindow(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	l := newLimiter(registerPerMinute, time.Minute)
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
	l := newLimiter(1, time.Minute)
	if !l.allow("a") || l.allow("a") {
		t.Fatal("key a should be allowed once")
	}
	if !l.allow("b") {
		t.Fatal("key b was refused because of key a")
	}
}
