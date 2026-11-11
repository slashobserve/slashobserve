package main

import (
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestViewersDashboardIsAValidSpec(t *testing.T) {
	if _, err := spec.Parse(viewersSpec); err != nil {
		t.Fatalf("the viewers dashboard does not parse: %v", err)
	}
}
