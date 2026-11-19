package logpatterns

import (
	"strconv"
	"testing"
)

func TestClusterSaysItWasTruncatedWhenLinesPastTheLineCapWereDropped(t *testing.T) {
	texts := make([]string, MaxLines+1)
	for i := range texts {
		texts[i] = "same line"
	}

	if _, truncated := Cluster(lines(texts...), 10, window.from, window.to); !truncated {
		t.Fatal("truncated = false, want true")
	}
}

func TestClusterSaysItWasTruncatedWhenLinesWereFoldedPastTheClusterCap(t *testing.T) {
	var texts []string
	for i := range MaxClusters {
		texts = append(texts, "kind"+strconv.Itoa(i)+" fixed")
	}
	texts = append(texts, "wholly different line here")

	if _, truncated := Cluster(lines(texts...), MaxClusters+10, window.from, window.to); !truncated {
		t.Fatal("truncated = false, want true")
	}
}

func TestClusterIsNotTruncatedWhenNothingWasCutOrFolded(t *testing.T) {
	if _, truncated := Cluster(lines("a b c", "a b d"), 10, window.from, window.to); truncated {
		t.Fatal("truncated = true, want false")
	}
}
