package auditsource

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/slashobserve/slashobserve/internal/audit"
	"github.com/slashobserve/slashobserve/internal/audit/filter"
	"github.com/slashobserve/slashobserve/internal/audit/trail"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/datasource/elasticsearch"
	"github.com/slashobserve/slashobserve/internal/logattrs"
	"github.com/slashobserve/slashobserve/internal/logvolume"
)

const (
	UID                  = "slashobserve-audit"
	Name                 = "Audit trail"
	Index                = "audit_events"
	TimeField            = "occurred_at"
	MessageField         = "message"
	LevelField           = "outcome"
	MaxConcurrentQueries = 4

	unreachableURL = "http://audit.invalid"
	resultWindow   = 10000
	maxRequestBody = 1 << 20
)

var outcomeLevels = map[string]logvolume.Kind{
	string(audit.OutcomeFailed):  logvolume.Error,
	string(audit.OutcomeDenied):  logvolume.Warning,
	string(audit.OutcomeSuccess): logvolume.Info,
}

type Store interface {
	Rows(ctx context.Context, q filter.Query, w filter.Window, page filter.Page) ([]trail.Event, error)
	Count(ctx context.Context, q filter.Query, w filter.Window, upTo int) (int64, error)
	ReadAggregation(ctx context.Context, p trail.Prepared) (trail.Aggregation, error)
}

func Instance(store Store, logger *slog.Logger) (datasource.Instance, error) {
	ds, err := elasticsearch.New(elasticsearch.Options{
		Flavor:          elasticsearch.OpenSearch,
		URL:             unreachableURL,
		Index:           Index,
		TimeField:       TimeField,
		QueryLanguage:   elasticsearch.QueryLanguageKQL,
		Fields:          elasticsearch.FieldRoles{Message: MessageField, Level: LevelField, TraceID: "trace_id"},
		Limits:          datasource.Limits{Timeout: trail.StatementTimeout, MaxRows: filter.MaxRows},
		MaxAggregations: maxAggregations,
		KnownField:      filter.Known,
		LevelKinds:      outcomeLevels,
		HTTPClient:      &http.Client{Transport: &emulator{store: store, logger: logger}},
	})
	if err != nil {
		return datasource.Instance{}, err
	}
	return datasource.Instance{UID: UID, Name: Name, Datasource: ds}, nil
}

type emulator struct {
	store  Store
	logger *slog.Logger
}

func (e *emulator) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(io.LimitReader(req.Body, maxRequestBody+1))
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
	}
	ctx := req.Context()
	status, out, err := e.serve(ctx, req.Method, req.URL.Path, body)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		status, out = e.failure(ctx, err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		Status:        http.StatusText(status),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": {"application/json"}},
		Body:          io.NopCloser(bytes.NewReader(raw)),
		ContentLength: int64(len(raw)),
		Request:       req,
	}, nil
}

func (e *emulator) serve(ctx context.Context, method, path string, body []byte) (int, any, error) {
	if len(body) > maxRequestBody {
		return 0, nil, refuse("the request body is larger than %d bytes", maxRequestBody)
	}
	indexPath := "/" + Index
	switch {
	case method == http.MethodGet && path == "/":
		return http.StatusOK, rootInfo, nil
	case method == http.MethodGet && path == "/_resolve/index/"+Index:
		return http.StatusOK, map[string]any{"indices": []any{map[string]any{"name": Index, "attributes": []string{"open"}}}, "aliases": []any{}, "data_streams": []any{}}, nil
	case method == http.MethodGet && path == indexPath+"/_settings":
		return http.StatusOK, map[string]any{Index: map[string]any{"settings": map[string]any{"index": map[string]any{"max_result_window": fmt.Sprint(resultWindow)}}}}, nil
	case method == http.MethodPost && path == indexPath+"/_field_caps":
		return http.StatusOK, fieldCaps(), nil
	case method == http.MethodPost && path == indexPath+"/_validate/query":
		out, err := validate(body)
		return http.StatusOK, out, err
	case method == http.MethodPost && path == indexPath+"/_search":
		out, err := e.search(ctx, body)
		return http.StatusOK, out, err
	}
	return 0, nil, refuse("the audit datasource does not support %s %q", method, path)
}

var rootInfo = map[string]any{
	"name":         "slashobserve",
	"cluster_name": "slashobserve-audit",
	"version":      map[string]any{"distribution": "opensearch", "number": "2.19.0"},
	"tagline":      "The audit trail of this SlashObserve server",
}

type refusal struct {
	status int
	kind   string
	reason string
}

func (r *refusal) Error() string { return r.reason }

func refuse(format string, args ...any) error {
	return &refusal{status: http.StatusBadRequest, kind: "illegal_argument_exception", reason: fmt.Sprintf(format, args...)}
}

func tooManyBuckets(format string, args ...any) error {
	return &refusal{status: http.StatusBadRequest, kind: "too_many_buckets_exception", reason: fmt.Sprintf(format, args...)}
}

func errorBody(r *refusal) map[string]any {
	cause := map[string]any{"type": r.kind, "reason": r.reason}
	return map[string]any{"error": map[string]any{"type": r.kind, "reason": r.reason, "root_cause": []any{cause}}, "status": r.status}
}

func (e *emulator) failure(ctx context.Context, err error) (int, any) {
	var r *refusal
	if errors.As(err, &r) {
		return r.status, errorBody(r)
	}
	if problem, ok := trail.ProblemOf(err); ok {
		if problem.Code == trail.CodeQueryTimeout {
			return http.StatusGatewayTimeout, errorBody(&refusal{status: http.StatusGatewayTimeout, kind: "search_timeout_exception", reason: problem.Message})
		}
		return http.StatusBadRequest, errorBody(&refusal{status: http.StatusBadRequest, kind: "illegal_argument_exception", reason: problem.Message})
	}
	e.logger.ErrorContext(ctx, "audit datasource read failed", logattrs.Failure("audit_datasource_read_failed", err)...)
	return http.StatusInternalServerError, errorBody(&refusal{status: http.StatusInternalServerError, kind: "exception", reason: "the audit trail could not be read"})
}
