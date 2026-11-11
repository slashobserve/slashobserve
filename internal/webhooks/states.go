package webhooks

import "github.com/slashobserve/slashobserve/internal/lifecycle"

const (
	StateDisabled = "disabled"
	StateRetrying = "retrying"
	StateSending  = "sending"
)

var States = lifecycle.Derived{Name: "webhook", States: []lifecycle.DerivedState{
	{Name: StateDisabled, When: "An admin turned it off, or the server did because the receiver is gone, deliveries kept failing, its owner is no longer an admin or its secret cannot be read; disabled_reason says which."},
	{Name: StateRetrying, When: "The last delivery failed and the server retries in order."},
	{Name: StateSending, When: "None of the above: deliveries go out as events happen."},
}}

func stateOf(enabled bool, consecutiveFailures int) string {
	switch {
	case !enabled:
		return StateDisabled
	case consecutiveFailures > 0:
		return StateRetrying
	}
	return StateSending
}
