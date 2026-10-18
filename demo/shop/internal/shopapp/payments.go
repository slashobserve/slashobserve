package shopapp

import (
	"net/http"
	"time"
)

const (
	declineRate = 0.02
	gatewayRate = 0.003
)

var declineReasons = [...]string{"card_expired", "insufficient_funds"}

func (a *app) charge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, span, ok := a.work(ctx, "authorize_card", 30*time.Millisecond, 70*time.Millisecond)
	defer span.End()
	if !ok {
		return
	}

	switch {
	case a.rng.Chance(declineRate):
		reason := declineReasons[a.alternate.Add(1)%uint64(len(declineReasons))]
		a.log.WarnContext(ctx, "card declined", "decline.reason", reason)
		failSpan(span, "card declined")
		writeJSON(w, http.StatusPaymentRequired, map[string]string{"error": "card declined", "reason": reason})
	case a.rng.Chance(gatewayRate):
		a.log.ErrorContext(ctx, "payment gateway unavailable")
		failSpan(span, "payment gateway unavailable")
		writeError(w, http.StatusServiceUnavailable, "payment gateway unavailable")
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "authorized"})
	}
}
