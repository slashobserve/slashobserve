package config

import (
	"testing"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

func TestADatasourceAudienceIsReadFromTheFile(t *testing.T) {
	cases := map[string]datasource.Audience{
		"":                   "",
		"audience: everyone": datasource.AudienceEveryone,
		"audience: admins":   datasource.AudienceAdmins,
	}
	for line, want := range cases {
		t.Run(line, func(t *testing.T) {
			got, err := load(t, entry("prom", "prometheus", promBlock+"\n"+line), nil)
			if err != nil {
				t.Fatal(err)
			}
			if got[0].Audience != want {
				t.Fatalf("audience = %q, want %q", got[0].Audience, want)
			}
		})
	}
}

func TestADatasourceAudienceMustBeEveryoneOrAdmins(t *testing.T) {
	wantError(t, entry("prom", "prometheus", promBlock+"\naudience: Admins"), nil, `datasources[0].audience: must be everyone or admins, got "Admins"`)
	wantError(t, entry("prom", "prometheus", promBlock+"\naudience: [admins]"), nil, "datasources[0].audience: must be a string")
}
