package server

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/gin-gonic/gin"

	"github.com/slashobserve/slashobserve/api"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	apiPrefix            = "/api/v1"
	eventStreamMediaType = "text/event-stream"
	contractViolation    = "api contract violation"

	// Must stay above every handler's own body limit (the largest is the dashboards' spec.MaxBytes plus 64 KiB),
	// or that handler's 413 can never be reached.
	maxContractBodyBytes = 4 * spec.MaxBytes
)

var defineFormats = sync.OnceFunc(func() {
	openapi3.DefineStringFormatValidator("uuid", openapi3.NewRegexpFormatValidator(openapi3.FormatOfStringForUUIDOfRFC9562))
})

type contract struct {
	router  routers.Router
	logger  *slog.Logger
	options *openapi3filter.Options
	doc     *openapi3.T
}

func newContract(ctx context.Context, logger *slog.Logger) (*contract, error) {
	defineFormats()
	doc, err := api.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load api document: %w", err)
	}
	router, err := legacy.NewRouter(doc)
	if err != nil {
		return nil, fmt.Errorf("build api router: %w", err)
	}
	return &contract{
		router: router,
		logger: logger,
		doc:    doc,
		options: &openapi3filter.Options{
			AuthenticationFunc:    openapi3filter.NoopAuthenticationFunc,
			IncludeResponseStatus: true,
		},
	}, nil
}

func (ct *contract) middleware(c *gin.Context) {
	ctx := c.Request.Context()
	route, pathParams, err := ct.router.FindRoute(c.Request)
	if err != nil {
		ct.violation(c, nil, "no documented operation", 0)
		return
	}
	input := &openapi3filter.RequestValidationInput{
		Request:    c.Request,
		PathParams: pathParams,
		Route:      route,
		Options:    ct.options,
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxContractBodyBytes+1))
	if err != nil {
		ct.violation(c, route, "request body is unreadable", 0)
		return
	}
	if len(body) > maxContractBodyBytes {
		c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body is too large", "code": "too_large"})
		return
	}
	restoreBody(c.Request, body)
	requestErr := openapi3filter.ValidateRequest(ctx, input)
	restoreBody(c.Request, body)
	if requestErr == nil && route.Operation.RequestBody == nil && len(body) > 0 {
		requestErr = &openapi3filter.RequestError{Reason: "request body is not allowed for this operation"}
	}

	if streamsEvents(route.Operation) {
		c.Next()
		ct.logAcceptedInvalidRequest(c, route, requestErr)
		return
	}

	original := c.Writer
	buffered := &bufferedWriter{ResponseWriter: original, header: original.Header().Clone(), status: http.StatusOK}
	c.Writer = buffered
	defer func() { c.Writer = original }()
	c.Next()
	c.Writer = original

	if ctx.Err() != nil {
		original.WriteHeader(buffered.status)
		original.WriteHeaderNow()
		return
	}
	if requestErr != nil && !isClientError(buffered.status) {
		ct.violation(c, route, fmt.Sprintf("request breaks the document but was accepted with status %d: %s", buffered.status, describe(requestErr)), buffered.status)
		return
	}
	responseInput := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: input,
		Status:                 buffered.status,
		Header:                 buffered.header,
		Options:                ct.options,
	}
	responseInput.SetBodyBytes(buffered.body.Bytes())
	if err := openapi3filter.ValidateResponse(ctx, responseInput); err != nil {
		ct.violation(c, route, describe(err), buffered.status)
		return
	}
	clear(original.Header())
	maps.Copy(original.Header(), buffered.header)
	original.WriteHeader(buffered.status)
	_, _ = original.Write(buffered.body.Bytes())
}

func restoreBody(r *http.Request, body []byte) {
	if len(body) == 0 {
		r.Body = http.NoBody
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
}

func (ct *contract) logAcceptedInvalidRequest(c *gin.Context, route *routers.Route, requestErr error) {
	if requestErr != nil && !isClientError(c.Writer.Status()) {
		ct.logger.ErrorContext(c.Request.Context(), contractViolation,
			slog.String("method", route.Method),
			slog.String("route", route.Path),
			slog.Int("status", c.Writer.Status()),
			slog.String("reason", describe(requestErr)))
	}
}

func (ct *contract) violation(c *gin.Context, route *routers.Route, reason string, status int) {
	attrs := []any{slog.String("method", c.Request.Method), slog.String("reason", reason)}
	if status != 0 {
		attrs = append(attrs, slog.Int("status", status))
	}
	if route != nil {
		attrs = append(attrs, slog.String("route", route.Path))
	} else {
		attrs = append(attrs, slog.String("route", c.FullPath()))
	}
	ct.logger.ErrorContext(c.Request.Context(), contractViolation, attrs...)
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.Writer.WriteHeader(http.StatusInternalServerError)
	_, _ = c.Writer.WriteString(`{"error":"api contract violation","code":"contract_violation"}`)
	c.Abort()
}

func (ct *contract) checkRoutes(engine *gin.Engine) error {
	registered := map[string]bool{}
	for _, r := range engine.Routes() {
		if strings.HasPrefix(r.Path, apiPrefix+"/") {
			registered[r.Method+" "+ginToTemplate(r.Path)] = true
		}
	}
	documented := map[string]bool{}
	for path, item := range ct.doc.Paths.Map() {
		for method := range item.Operations() {
			documented[method+" "+apiPrefix+path] = true
		}
	}
	var undocumented, unimplemented []string
	for key := range registered {
		if !documented[key] {
			undocumented = append(undocumented, key)
		}
	}
	for key := range documented {
		if !registered[key] {
			unimplemented = append(unimplemented, key)
		}
	}
	if len(undocumented) == 0 && len(unimplemented) == 0 {
		return nil
	}
	slices.Sort(undocumented)
	slices.Sort(unimplemented)
	return fmt.Errorf("api contract: undocumented routes %v, unimplemented operations %v", undocumented, unimplemented)
}

func ginToTemplate(path string) string {
	segments := strings.Split(path, "/")
	for i, s := range segments {
		if name, ok := strings.CutPrefix(s, ":"); ok {
			segments[i] = "{" + name + "}"
		}
	}
	return strings.Join(segments, "/")
}

func isClientError(status int) bool { return status >= 400 && status < 500 }

func streamsEvents(op *openapi3.Operation) bool {
	for _, status := range []int{http.StatusOK, http.StatusCreated} {
		if ref := op.Responses.Status(status); ref != nil && ref.Value != nil && ref.Value.Content.Get(eventStreamMediaType) != nil {
			return true
		}
	}
	return false
}

// Response-side reasons are fixed texts from kin-openapi that interpolate only a MIME type, a header name or a schema
// id, and the fields of a response are written by the server. A request carries client input, so its reason is withheld.
func describe(err error) string {
	var parts []string
	var requestErr *openapi3filter.RequestError
	var responseErr *openapi3filter.ResponseError
	var schemaErr *openapi3.SchemaError
	isResponse := false
	switch {
	case errors.As(err, &responseErr):
		isResponse = true
		parts = append(parts, reasonOrFallback(responseErr.Reason, "response does not match the document", responseErr.Err))
	case errors.As(err, &requestErr):
		if p := requestErr.Parameter; p != nil {
			parts = append(parts, "parameter "+p.In+" "+p.Name)
		}
		parts = append(parts, causeOrFallback("request does not match the document", requestErr.Err))
	default:
		parts = append(parts, "validation failed")
	}
	if errors.As(err, &schemaErr) {
		parts = append(parts, "/"+strings.Join(schemaErr.JSONPointer(), "/")+": "+schemaErr.SchemaField)
		if extra := undocumentedFields(schemaErr); isResponse && len(extra) > 0 {
			parts = append(parts, "undocumented fields "+strings.Join(extra, ", "))
		}
	}
	return strings.Join(parts, ": ")
}

const maxReportedFields = 10

func undocumentedFields(schemaErr *openapi3.SchemaError) []string {
	object, ok := schemaErr.Value.(map[string]any)
	if schemaErr.SchemaField != "properties" || !ok || schemaErr.Schema == nil {
		return nil
	}
	var names []string
	for name := range object {
		if _, documented := schemaErr.Schema.Properties[name]; !documented {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names[:min(len(names), maxReportedFields)]
}

func reasonOrFallback(reason, fallback string, cause error) string {
	if reason != "" {
		return reason
	}
	return causeOrFallback(fallback, cause)
}

func causeOrFallback(fallback string, cause error) string {
	if cause != nil {
		return fmt.Sprintf("%s (%T)", fallback, cause)
	}
	return fallback
}

type bufferedWriter struct {
	gin.ResponseWriter
	header  http.Header
	status  int
	body    bytes.Buffer
	written bool
}

func (w *bufferedWriter) Header() http.Header { return w.header }

func (w *bufferedWriter) WriteHeader(code int) {
	if !w.written {
		w.status = code
	}
}

func (w *bufferedWriter) WriteHeaderNow() { w.written = true }

func (w *bufferedWriter) Write(b []byte) (int, error) {
	w.written = true
	return w.body.Write(b)
}

func (w *bufferedWriter) WriteString(s string) (int, error) {
	w.written = true
	return w.body.WriteString(s)
}

func (w *bufferedWriter) Status() int { return w.status }

func (w *bufferedWriter) Size() int { return w.body.Len() }

func (w *bufferedWriter) Written() bool { return w.written }

func (w *bufferedWriter) Flush() {}

func (w *bufferedWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("hijack is not available while the api contract check buffers responses")
}
