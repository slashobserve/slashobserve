package queryproxy

import (
	"testing"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func TestNamedTimeZoneIsOnlyAnIANANameTheServerCanUse(t *testing.T) {
	for _, tc := range []struct{ zone, want string }{
		{"", ""},
		{spec.TimezoneUTC, ""},
		{spec.TimezoneBrowser, ""},
		{"Asia/Kolkata", "Asia/Kolkata"},
	} {
		if got := namedTimeZone(&spec.Dashboard{Timezone: tc.zone}); got != tc.want {
			t.Errorf("zone %q = %q, want %q", tc.zone, got, tc.want)
		}
	}
}
