package webhooks

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/slashobserve/slashobserve/internal/egress"
)

const (
	outcomeDelivered = "delivered"
	outcomeFailed    = "failed"
	outcomeSkipped   = "skipped"

	errorStatus           = "status"
	errorUnreachable      = "unreachable"
	errorBlockedAddress   = "blocked_address"
	errorDomainNotAllowed = "domain_not_allowed"
	errorTooLarge         = "too_large"
)

type Sender interface {
	Send(ctx context.Context, req egress.SendRequest) (egress.SendResult, error)
}

type attempt struct {
	outcome    string
	errorCode  string
	status     int
	duration   time.Duration
	retryAfter time.Duration
	gone       bool
}

func deliver(ctx context.Context, sender Sender, at time.Time, address, secret, deliveryID, eventType string, body []byte) (attempt, error) {
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	header.Set("X-SlashObserve-Event", eventType)
	header.Set("X-SlashObserve-Delivery", deliveryID)
	header.Set("X-SlashObserve-Signature", signature(secret, at, body))

	started := time.Now()
	result, err := sender.Send(ctx, egress.SendRequest{Method: http.MethodPost, URL: address, Header: header, Body: body})
	got := attempt{duration: time.Since(started), status: result.Status, retryAfter: result.RetryAfter}
	if err != nil {
		if ctx.Err() != nil {
			return attempt{}, ctx.Err()
		}
		got.status = 0
		got.outcome, got.errorCode = classifyError(err)
		return got, nil
	}
	switch {
	case result.Status >= 200 && result.Status < 300:
		got.outcome = outcomeDelivered
	case result.Status == http.StatusGone:
		got.outcome, got.errorCode, got.gone = outcomeFailed, errorStatus, true
	default:
		got.outcome, got.errorCode = outcomeFailed, errorStatus
	}
	return got, nil
}

func classifyError(err error) (outcome, code string) {
	switch {
	case errors.Is(err, egress.ErrTooLarge):
		return outcomeSkipped, errorTooLarge
	case errors.Is(err, egress.ErrBlockedAddress):
		return outcomeFailed, errorBlockedAddress
	case errors.Is(err, egress.ErrNotAllowed):
		return outcomeFailed, errorDomainNotAllowed
	}
	return outcomeFailed, errorUnreachable
}
