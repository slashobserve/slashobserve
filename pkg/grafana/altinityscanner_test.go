package grafana

import (
	"testing"
	"time"
)

func TestAltinityMacroQueryWithNonASCIILetterTerminates(t *testing.T) {
	queries := []string{
		"$rate(count() c) FROM db.t WHERE d = 1 é",
		"$rate(count() c) FROM db.t WHERE d = 1 ü AND x > 2",
		"$columns(k, count() c) FROM $table WHERE n = 'é' и",
	}
	for _, query := range queries {
		done := make(chan int, 1)
		go func() {
			panel, _ := altinityPanel(t, query, "")
			done <- len(panel.Queries)
		}()
		select {
		case n := <-done:
			if n != 1 {
				t.Fatalf("%q: queries = %d, want 1", query, n)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("%q: conversion did not finish", query)
		}
	}
}
