package main

import (
	"strings"
	"testing"
)

func TestFaultTakesEveryShopFaultType(t *testing.T) {
	for _, kind := range []string{"errors", "latency", "queue", "slow_db"} {
		if err := validateFault(Fault{Pod: "checkout-0", Type: kind, Pct: 30}); err != nil {
			t.Errorf("fault type %s: %v", kind, err)
		}
	}
	err := validateFault(Fault{Pod: "checkout-0", Type: "slow", Pct: 30})
	if err == nil || !strings.Contains(err.Error(), `fault.type "slow" must be one of errors, latency, queue, slow_db`) {
		t.Fatalf("an unknown fault type: %v", err)
	}
}
