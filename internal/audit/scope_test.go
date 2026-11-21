package audit

import "testing"

func TestARequestIsARepeatOnlyWhenEveryReadInItWas(t *testing.T) {
	for name, tc := range map[string]struct {
		repeats []bool
		want    bool
	}{
		"no read":                 {nil, false},
		"one repeated read":       {[]bool{true}, true},
		"two repeated reads":      {[]bool{true, true}, true},
		"a repeat and a new read": {[]bool{true, false}, false},
		"a new read and a repeat": {[]bool{false, true}, false},
		"one new read":            {[]bool{false}, false},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := Open(t.Context(), Scope{})
			for _, repeated := range tc.repeats {
				NoteRead(ctx, repeated)
			}
			if got := stateOf(ctx).repeatsARead("dashboard.view", OutcomeSuccess); got != tc.want {
				t.Fatalf("repeatsARead = %v, want %v", got, tc.want)
			}
		})
	}
}
