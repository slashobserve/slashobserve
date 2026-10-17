package shopapp

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	invalidCartMessage = "invalid cart: empty items"
	malformedCartMsg   = "invalid cart: malformed json"
	requestTooLargeMsg = "invalid cart: request body too large"
	reserveTimeoutMsg  = "inventory reservation timed out"
	insertDeadlockMsg  = "order insert failed: deadlock detected"
	checkoutFailRate   = 0.005
)

type cart struct {
	Items []struct {
		SKU string `json:"sku"`
		Qty int    `json:"qty"`
	} `json:"items"`
}

type chargeResult struct {
	Reason string `json:"reason"`
}

func (a *app) checkout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var c cart
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err == nil {
		err = json.Unmarshal(body, &c)
	}
	if err != nil {
		a.writeBodyError(w, r, err, requestTooLargeMsg, malformedCartMsg)
		return
	}
	if len(c.Items) == 0 {
		a.log.WarnContext(ctx, invalidCartMessage)
		writeError(w, http.StatusBadRequest, invalidCartMessage)
		return
	}

	failure := ""
	if a.rng.Chance(checkoutFailRate) {
		failure = reserveTimeoutMsg
		if a.alternate.Add(1)%2 == 0 {
			failure = insertDeadlockMsg
		}
	}

	_, reserve, ok := a.work(ctx, "reserve_inventory", 10*time.Millisecond, 30*time.Millisecond)
	if failure == reserveTimeoutMsg {
		failSpan(reserve, failure)
	}
	reserve.End()
	if !ok {
		return
	}
	if failure == reserveTimeoutMsg {
		a.failCheckout(w, r, failure)
		return
	}

	if !a.authorizePayment(w, r, body) {
		return
	}

	_, insert, ok := a.work(ctx, "insert_order", 10*time.Millisecond, 30*time.Millisecond)
	if failure == insertDeadlockMsg {
		failSpan(insert, failure)
	}
	insert.End()
	if !ok {
		return
	}
	if failure == insertDeadlockMsg {
		a.failCheckout(w, r, failure)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"order_id": a.orderSeq.Add(1)})
}

func (a *app) authorizePayment(w http.ResponseWriter, r *http.Request, body []byte) bool {
	ctx := r.Context()
	status, payload, err := a.call(ctx, http.MethodPost, a.cfg.PaymentsURL+"/charge", body)
	if ctx.Err() != nil {
		return false
	}
	switch {
	case err != nil || status >= http.StatusInternalServerError:
		a.log.ErrorContext(ctx, "payment service unavailable", upstreamAttrs(a.cfg.PaymentsURL, status, err)...)
		writeError(w, http.StatusBadGateway, "payment service unavailable")
	case status == http.StatusPaymentRequired:
		var result chargeResult
		_ = json.Unmarshal(payload, &result)
		message := "payment declined: " + result.Reason
		a.log.ErrorContext(ctx, message)
		writeError(w, http.StatusPaymentRequired, message)
	case status >= http.StatusMultipleChoices:
		a.log.ErrorContext(ctx, "payment service unavailable", upstreamAttrs(a.cfg.PaymentsURL, status, nil)...)
		writeError(w, http.StatusBadGateway, "payment service unavailable")
	default:
		return true
	}
	return false
}

func (a *app) failCheckout(w http.ResponseWriter, r *http.Request, message string) {
	a.log.ErrorContext(r.Context(), message)
	writeError(w, http.StatusInternalServerError, message)
}

func (a *app) getOrder(w http.ResponseWriter, r *http.Request) {
	if !sleep(r.Context(), a.rng.Between(5*time.Millisecond, 20*time.Millisecond)) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid order id")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"order_id": id, "status": "shipped"})
}
