package inboundmcp

import (
	"errors"
	"testing"

	"github.com/slashobserve/slashobserve/internal/auth"
)

func TestApproveRequiresTheHashBeforeTouchingTheDatabase(t *testing.T) {
	_, err := (&Service{}).Approve(t.Context(), auth.Principal{}, "docs", "search", "")
	if !errors.Is(err, ErrToolHashRequired) {
		t.Fatalf("err = %v, want ErrToolHashRequired", err)
	}
}
