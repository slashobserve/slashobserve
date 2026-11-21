package audit_test

import (
	"slices"
	"testing"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/origin"
)

func storedValues() map[string][]string {
	return map[string][]string{
		"category": {
			string(audit.CategoryAuth), string(audit.CategoryAccess), string(audit.CategoryContent), string(audit.CategoryConversation),
			string(audit.CategoryAgent), string(audit.CategoryQuery), string(audit.CategoryAdmin), string(audit.CategorySystem),
		},
		"outcome": {
			string(audit.OutcomeSuccess), string(audit.OutcomeDenied), string(audit.OutcomeFailed), string(audit.OutcomeUnknown),
		},
		"actor_kind": {
			string(audit.ActorUser), string(audit.ActorServiceAccount), string(audit.ActorAnonymous), string(audit.ActorSystem),
		},
		"initiator": {
			string(audit.InitiatorPerson), string(audit.InitiatorAgent), string(audit.InitiatorSystem),
		},
		"auth_method": {
			string(audit.AuthSession), string(audit.AuthAPIKey), string(audit.AuthOAuth), string(audit.AuthPublicLink), string(audit.AuthNone),
		},
		"origin": {
			audit.OriginWeb, audit.OriginAPI, audit.OriginJob, audit.OriginAgent, audit.OriginUnknown,
			string(origin.Web), string(origin.CLI), string(origin.MCP), string(origin.API),
		},
	}
}

func TestEveryValueAFacetCanReturnIsAValueTheFilterAccepts(t *testing.T) {
	stored := storedValues()
	for _, field := range filter.Fields() {
		if field.Type != filter.TypeEnum {
			continue
		}
		values, ok := stored[field.Name]
		if !ok {
			t.Errorf("the enumerated field %s has no list of the values the trail stores", field.Name)
			continue
		}
		for _, value := range values {
			if _, err := filter.Parse(field.Name + ":" + value); err != nil {
				t.Errorf("%s:%s is stored but is refused as a filter: %v", field.Name, value, err)
			}
			if !slices.Contains(field.Values, value) {
				t.Errorf("%s lists %v and lacks the stored value %s", field.Name, field.Values, value)
			}
		}
	}
}
