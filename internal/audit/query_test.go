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

func TestARecordedQueryMasksTheUserinfoOfAnyURLBeforeItIsCut(t *testing.T) {
	for in, want := range map[string]string{
		"SELECT * FROM url('https://user:pw@host/x.csv')":     "SELECT * FROM url('https://***@host/x.csv')",
		`{app="a"} |= "postgres://admin:p@ss@db:5432/app"`:    `{app="a"} |= "postgres://***@db:5432/app"`,
		"https://host/path?next=https://a:b@other/ and x@y.z": "https://host/path?next=https://***@other/ and x@y.z",
		"https://host/no-userinfo and mail me@example.com":    "https://host/no-userinfo and mail me@example.com",
		"up == 0": "up == 0",
	} {
		if got := RecordedQuery(in); got != want {
			t.Errorf("RecordedQuery(%q) = %q, want %q", in, got, want)
		}
	}
	secret := strings.Repeat("a", MaxQueryBytes-8) + "s3://k:SECRETKEY@bucket"
	if got := RecordedQuery(secret); strings.Contains(got, "SECRETKEY") {
		t.Error("a credential that straddles the cap survived")
	}
}
