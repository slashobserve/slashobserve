package shopapp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	catalogSize       = 20
	sellableProducts  = 18
	maxRequestBody    = 1 << 20
	maxUpstreamBody   = 1 << 20
	jsonContentType   = "application/json"
	headerContentType = "Content-Type"
)

type product struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Price int    `json:"price_cents"`
}

var catalog = func() []product {
	products := make([]product, catalogSize)
	for i := range products {
		products[i] = product{ID: i + 1, Name: fmt.Sprintf("Product %d", i+1), Price: 499 + 250*i}
	}
	return products
}()

func (a *app) listProducts(w http.ResponseWriter, r *http.Request) {
	_, span, ok := a.work(r.Context(), "load_catalog", 5*time.Millisecond, 30*time.Millisecond)
	span.End()
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": catalog})
}

func (a *app) getProduct(w http.ResponseWriter, r *http.Request) {
	if !sleep(r.Context(), a.rng.Between(5*time.Millisecond, 30*time.Millisecond)) {
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id < 1 || id > sellableProducts {
		a.log.WarnContext(r.Context(), "product not found", "product.id", r.PathValue("id"))
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	writeJSON(w, http.StatusOK, catalog[id-1])
}

func (a *app) frontendCheckout(w http.ResponseWriter, r *http.Request) {
	if !sleep(r.Context(), a.rng.Between(2*time.Millisecond, 8*time.Millisecond)) {
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		a.writeBodyError(w, r, err, "request body too large", "unreadable request body")
		return
	}
	a.forwardToCheckout(w, r, http.MethodPost, "/checkout", body)
}

func (a *app) frontendOrder(w http.ResponseWriter, r *http.Request) {
	if !sleep(r.Context(), a.rng.Between(2*time.Millisecond, 5*time.Millisecond)) {
		return
	}
	a.forwardToCheckout(w, r, http.MethodGet, "/orders/"+url.PathEscape(r.PathValue("id")), nil)
}

func (a *app) forwardToCheckout(w http.ResponseWriter, r *http.Request, method, path string, body []byte) {
	n := uint64(len(a.cfg.CheckoutURLs))
	base := a.cfg.CheckoutURLs[(a.rotation.Add(1)-1)%n]
	status, payload, err := a.call(r.Context(), method, base+path, body)
	if r.Context().Err() != nil {
		return
	}
	if err != nil || status >= http.StatusInternalServerError {
		a.log.ErrorContext(r.Context(), "upstream checkout failed", upstreamAttrs(base, status, err)...)
		writeError(w, http.StatusBadGateway, "checkout unavailable")
		return
	}
	w.Header().Set(headerContentType, jsonContentType)
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func (a *app) call(ctx context.Context, method, target string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body)) //nolint:gosec // target is an operator-configured upstream base URL plus a fixed or escaped path
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		req.Header.Set(headerContentType, jsonContentType)
	}
	resp, err := a.client.Do(req) //nolint:gosec // request target is built from operator-configured upstream URLs
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamBody))
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, payload, nil
}

func upstreamAttrs(target string, status int, err error) []any {
	attrs := []any{"upstream.url", target}
	if status != 0 {
		attrs = append(attrs, "upstream.status_code", status)
	}
	if err != nil {
		attrs = append(attrs, "error", err.Error())
	}
	return attrs
}
