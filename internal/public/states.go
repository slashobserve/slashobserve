package public

import "github.com/slashobserve/slashobserve/internal/lifecycle"

const (
	LinkStateExpired = "expired"
	LinkStateActive  = "active"
)

var LinkStates = lifecycle.Derived{Name: "public link", States: []lifecycle.DerivedState{
	{Name: LinkStateExpired, When: "Its expiry time has passed; the link no longer opens."},
	{Name: LinkStateActive, When: "None of the above; usable says whether the link opens, and unusable_reason says why it does not."},
}}

var UnusableReasons = []string{reasonExpired, reasonAdminsOnlyFolder, reasonUserScoped, reasonRestrictedDatasource}

func linkState(expired bool) string {
	if expired {
		return LinkStateExpired
	}
	return LinkStateActive
}
