package audit

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/logattrs"
)

const (
	RequestIDHeader = "X-Request-Id"
	maxErrorBody    = 4096
	traceparentLen  = 55
)

func (r *Recorder) Middleware() (gin.HandlerFunc, error) {
	routes, err := declaredRoutes()
	if err != nil {
		return nil, fmt.Errorf("read %s declarations: %w", Extension, err)
	}
	return func(c *gin.Context) {
		r.inflight.Add(1)
		defer r.inflight.Add(-1)
		start := time.Now()
		template := ginToTemplate(c.FullPath())
		method := c.Request.Method
		if method == http.MethodHead {
			method = http.MethodGet
		}
		scope := Scope{
			RequestID: uuid.Must(uuid.NewV7()),
			TraceID:   traceID(c.GetHeader("traceparent")),
			Actor:     Actor{Kind: ActorAnonymous},
			Initiator: InitiatorPerson,
			Auth:      Auth{Method: AuthNone},
			Origin:    Origin{Kind: unauthenticatedOrigin(c.Request)},
			IP:        requestAddr(c),
			UserAgent: c.Request.UserAgent(),
			Method:    method,
			Route:     template,
		}
		ctx := Open(c.Request.Context(), scope)
		c.Request = c.Request.WithContext(ctx)
		c.Header(RequestIDHeader, scope.RequestID.String())
		capture := &errorCapture{ResponseWriter: c.Writer}
		c.Writer = capture
		recovered := next(c)
		declared, ok := routes.lookup(method, template)
		if !ok {
			declared = Declaration{}
		}
		refusal := capture.refusal()
		o := outcomeOf{
			declared: declared,
			status:   capture.Status(),
			code:     refusal.Code,
			needs:    refusal.Needs(),
			took:     time.Since(start),
			params:   c.Params,
			panicked: recovered != nil,
		}
		if o.panicked {
			o.status = http.StatusInternalServerError
		}
		r.settle(context.WithoutCancel(c.Request.Context()), stateOf(ctx), o)
		if recovered != nil {
			panic(recovered)
		}
	}, nil
}

func next(c *gin.Context) (recovered any) {
	defer func() { recovered = recover() }()
	c.Next()
	return nil
}

type outcomeOf struct {
	declared Declaration
	status   int
	code     string
	needs    map[string]any
	took     time.Duration
	params   gin.Params
	panicked bool
}

func (r *Recorder) settle(ctx context.Context, st *state, o outcomeOf) {
	committed, inTx, concluded := st.unsettled()
	if committed {
		return
	}
	action := o.declared.Action
	outcome := outcomeFor(o.status, o.declared)
	reason := o.code
	target := o.declared.target(o.params)
	var data map[string]any
	if concluded != nil {
		action = cmp.Or(concluded.Action, action)
		outcome, reason, target, data = concluded.Outcome, concluded.Reason, concluded.Target, concluded.Data
	}
	if o.panicked {
		outcome, reason = OutcomeFailed, "panic"
	}
	if outcome == OutcomeDenied {
		data = withNeeds(data, o.needs)
	}
	if len(inTx) > 0 {
		recorded, err := r.anyCommitted(ctx, inTx)
		switch {
		case err != nil:
			r.logger.WarnContext(ctx, "audit commit check failed", logattrs.Failure("audit_check_failed", err)...)
			outcome, reason = OutcomeUnknown, "commit_check_failed"
		case recorded:
			return
		default:
			outcome, reason = OutcomeFailed, cmp.Or(reason, "rolled_back")
		}
		action = cmp.Or(action, inTx[0].action)
	}
	if action == "" {
		if !refused(o.status) {
			return
		}
		action = ActionRefuse
	}
	if len(inTx) == 0 && st.repeatsARead(action, outcome) {
		return
	}
	data = st.withReadWindow(action, data)
	row, err := r.row(ctx, Event{
		Action:     action,
		Outcome:    outcome,
		Reason:     reason,
		Status:     o.status,
		Target:     target,
		Datasource: st.readDatasource(),
		Data:       data,
		Duration:   o.took,
	})
	if err != nil {
		r.logger.WarnContext(ctx, "audit request event not built", logattrs.Failure("audit_event_invalid", err)...)
		return
	}
	if batched(action, outcome) {
		r.writer.enqueue(row)
		return
	}
	if err := r.insertNow(ctx, row); err != nil {
		r.logger.WarnContext(ctx, "audit request event not written", logattrs.Failure("audit_record_failed", err)...)
		r.metrics.EventsDropped(DroppedRecordFailed, 1)
	}
}

func batched(action Action, outcome Outcome) bool {
	if outcome == OutcomeUnknown {
		return false
	}
	spec, _ := Lookup(action)
	return spec.Class == ClassAccess || outcome == OutcomeDenied
}

func (r *Recorder) anyCommitted(ctx context.Context, events []written) (bool, error) {
	times := make([]pgtype.Timestamptz, len(events))
	ids := make([]pgtype.UUID, len(events))
	for i, e := range events {
		times[i] = pgtype.Timestamptz{Time: e.at, Valid: true}
		ids[i] = pgtype.UUID{Bytes: e.id, Valid: true}
	}
	n, err := r.db.Queries().CountCommittedAuditEvents(ctx, dbsqlc.CountCommittedAuditEventsParams{OccurredAt: times, Ids: ids})
	return n > 0, err
}

func (d Declaration) target(params gin.Params) Target {
	if d.Target == nil {
		return Target{}
	}
	t := Target{Kind: d.Target.Kind}
	if name, ok := d.Target.PathParam(); ok {
		t.ID = params.ByName(name)
	}
	return t
}

func OutcomeForStatus(status int) Outcome {
	return outcomeFor(status, Declaration{})
}

func outcomeFor(status int, declared Declaration) Outcome {
	_, namesTarget := declared.Target.PathParam()
	switch {
	case redirected(status):
		return OutcomeUnknown
	case status < http.StatusBadRequest:
		return OutcomeSuccess
	case refused(status), status == http.StatusNotFound && namesTarget:
		return OutcomeDenied
	}
	return OutcomeFailed
}

func redirected(status int) bool {
	switch status {
	case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		return true
	}
	return false
}

func refused(status int) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests
}

func unauthenticatedOrigin(r *http.Request) string {
	if r.Header.Get("Authorization") != "" {
		return OriginAPI
	}
	return OriginWeb
}

func requestAddr(c *gin.Context) netip.Addr {
	if resolved := c.ClientIP(); resolved != "" {
		return clientAddr(resolved)
	}
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		host = c.Request.RemoteAddr
	}
	return clientAddr(host)
}

func clientAddr(address string) netip.Addr {
	addr, err := netip.ParseAddr(address)
	if err != nil {
		return netip.Addr{}
	}
	return addr.WithZone("").Unmap()
}

func traceID(traceparent string) string {
	parts := strings.Split(traceparent, "-")
	if len(traceparent) != traceparentLen || len(parts) != 4 || len(parts[1]) != 32 {
		return ""
	}
	for _, r := range parts[1] {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return ""
		}
	}
	if strings.Trim(parts[1], "0") == "" {
		return ""
	}
	return parts[1]
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

type errorCapture struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *errorCapture) Write(b []byte) (int, error) {
	w.keep(b)
	return w.ResponseWriter.Write(b)
}

func (w *errorCapture) WriteString(s string) (int, error) {
	w.keep([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

func (w *errorCapture) keep(b []byte) {
	if w.Status() < http.StatusBadRequest || w.body.Len() >= maxErrorBody {
		return
	}
	w.body.Write(b[:min(len(b), maxErrorBody-w.body.Len())])
}
