package datasource

import "github.com/slashobserve/slashobserve/internal/lifecycle"

type HealthState string

const (
	HealthUnchecked  HealthState = "unchecked"
	HealthHealthy    HealthState = "healthy"
	HealthFailing    HealthState = "failing"
	HealthRechecking HealthState = "rechecking"
)

var HealthStates = lifecycle.Derived{Name: "datasource health", States: []lifecycle.DerivedState{
	{Name: string(HealthUnchecked), When: "No check has answered: the caller left before it did, or the check was cancelled. `healthy` is `false`, and nothing says the datasource is failing."},
	{Name: string(HealthHealthy), When: "The last check passed, or the datasource answered a query after a check it did not answer."},
	{Name: string(HealthFailing), When: "The last check failed, and `failure_kind` says whether to wait or to change something."},
	{Name: string(HealthRechecking), When: "The last check failed 2 seconds ago or more and a new one is running; `message`, `detail` and `failure_kind` are still those of the failed check."},
}}

type FailureKind string

const (
	FailureTransient     FailureKind = "transient"
	FailureCredentials   FailureKind = "credentials"
	FailureConfiguration FailureKind = "configuration"
	FailureUnsupported   FailureKind = "unsupported"
	FailureUnknown       FailureKind = "unknown"

	notTheDatasource FailureKind = ""
)

var FailureKinds = []FailureKind{FailureTransient, FailureCredentials, FailureConfiguration, FailureUnsupported, FailureUnknown}
