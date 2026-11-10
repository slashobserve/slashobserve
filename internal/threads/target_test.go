package threads

import (
	"errors"
	"testing"
)

const sampleID = "7f0c1a2b-3c4d-4e5f-8a9b-0c1d2e3f4a5b"

func TestParseTargetRoundTrips(t *testing.T) {
	for _, kind := range []Kind{KindDashboard, KindFinding, KindAnnotation} {
		raw := string(kind) + ":" + sampleID
		got, err := ParseTarget(raw)
		if err != nil {
			t.Fatalf("ParseTarget(%q): %v", raw, err)
		}
		if got.Kind != kind || got.ID.String() != sampleID || got.String() != raw {
			t.Errorf("ParseTarget(%q) = %+v, String %q", raw, got, got.String())
		}
	}
}

func TestParseTargetRejectsMalformedTargets(t *testing.T) {
	for _, raw := range []string{
		"",
		"finding",
		"finding:",
		":" + sampleID,
		"run:" + sampleID,
		"Finding:" + sampleID,
		"finding/" + sampleID,
		"finding:not-a-uuid",
		"finding:7f0c1a2b3c4d4e5f8a9b0c1d2e3f4a5b",
		"finding:" + sampleID + ":extra",
	} {
		_, err := ParseTarget(raw)
		var invalid *InvalidInputError
		if !errors.As(err, &invalid) || invalid.Code != CodeInvalidTarget {
			t.Errorf("ParseTarget(%q) error = %v, want invalid_target", raw, err)
		}
	}
}
