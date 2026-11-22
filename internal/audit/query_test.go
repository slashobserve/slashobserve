package audit

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestARecordedQueryIsCutAtARuneBoundary(t *testing.T) {
	if got := RecordedQuery("outcome:denied"); got != "outcome:denied" {
		t.Errorf("a short query became %q", got)
	}
	for _, char := range []string{"é", "€", "😀"} {
		for lead := MaxQueryBytes - len(char); lead <= MaxQueryBytes; lead++ {
			long := strings.Repeat("a", lead) + char + "tail"
			got := RecordedQuery(long)
			kept := strings.TrimSuffix(got, truncatedMark)
			if !utf8.ValidString(got) || len(got) > MaxQueryBytes+len(truncatedMark) || kept == got || !strings.HasPrefix(long, kept) || len(kept) < MaxQueryBytes-len(char)+1 {
				t.Errorf("%q after %d bytes became %d bytes ending %q", char, lead, len(got), got[len(got)-20:])
			}
		}
	}
}
