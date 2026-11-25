package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/slashobserve/slashobserve/internal/accessrequests"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/httpapi"
)

const (
	nameRequestCommentAccess = "request_comment_access"
	nameListAccessRequests   = "list_access_requests"

	descRequestCommentAccess = "Ask for Comment access to a dashboard or observation that the caller can view but not comment on. It changes no access: it puts a request, with the optional one-line note, in the inbox of the people who manage the sharing of its folder (the folder's creator and anyone with Admin on it, or the workspace Admins when there are none), who grant it by changing the folder's access or the caller's role. The answer never names them. One open request per caller and object: asking again while one is open answers request_open, a caller who can already comment gets already_can_comment, and an object the caller cannot view is not_found. A request closes when access is granted or a granter marks its inbox item done, and after 30 days."
	descListAccessRequests   = "List the open requests for Comment access on dashboards and observations whose folder sharing the caller manages, newest first, at most 100: who asked, on what (kind, id, title), in which folder, the note and when. Grant one by giving the person Comment on the folder (folder permissions) or by raising their role; nothing here changes access. When folder.general is true the object is in General, which is open to everyone by role and takes no folder entry, so only a role change or moving the object to another folder first can grant it."

	propAccessNote = `"note":{"type":"string","maxLength":200,"description":"Optional one-line note for the people who can grant access, at most 200 characters, no line breaks."}`

	schemaRequestCommentAccess = `{"type":"object","properties":{` +
		`"object_kind":{"type":"string","enum":["dashboard","observation"],"description":"What the object is."},` +
		`"object_id":{"type":"string","description":"The dashboard or observation id."},` +
		propAccessNote + `},"required":["object_kind","object_id"],"additionalProperties":false}`
)

var errAccessRequestsNotConfigured = errors.New("access requests are not configured")

func init() { registerEntries((*Registry).accessRequestEntries) }

func (r *Registry) accessRequestEntries() []entry {
	return []entry{
		{Tool{Name: nameRequestCommentAccess, Description: descRequestCommentAccess, InputSchema: json.RawMessage(schemaRequestCommentAccess), MinRole: auth.RoleViewer}, r.requestCommentAccess},
		{Tool{Name: nameListAccessRequests, Description: descListAccessRequests, InputSchema: json.RawMessage(schemaEmpty), ReadOnly: true, MinRole: auth.RoleViewer}, r.listAccessRequests},
	}
}

type requestOpenBody struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	RequestedAt string `json:"requested_at"`
}

func (r *Registry) requestCommentAccess(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.AccessRequests == nil {
		return Result{}, errAccessRequestsNotConfigured
	}
	var in struct {
		ObjectKind string `json:"object_kind"`
		ObjectID   string `json:"object_id"`
		Note       string `json:"note"`
	}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	kind, ok := accessrequests.ParseKind(in.ObjectKind)
	if !ok {
		return invalid("object_kind must be dashboard or observation"), nil
	}
	id, res, ok := parseDashboardID(in.ObjectID)
	if kind == accessrequests.KindObservation {
		id, res, ok = parseObservationID(in.ObjectID)
	}
	if !ok {
		return res, nil
	}
	principal, _ := auth.FromContext(ctx)
	request, err := r.deps.AccessRequests.Request(ctx, principal, accessrequests.Object{Kind: kind, ID: id}, in.Note)
	var open *accessrequests.OpenError
	switch {
	case errors.Is(err, accessrequests.ErrInvalidNote):
		return toolError(accessrequests.CodeInvalidNote, err.Error()), nil
	case errors.Is(err, accessrequests.ErrNotFound):
		return toolError(codeNotFound, string(kind)+" not found, or you cannot see it"), nil
	case errors.Is(err, accessrequests.ErrCanComment):
		return toolError(accessrequests.CodeCanComment, accessrequests.CanCommentMessage), nil
	case errors.As(err, &open):
		return failure("", requestOpenBody{Code: accessrequests.CodeRequestOpen, Message: open.Message(), RequestedAt: httpapi.FormatTime(open.RequestedAt)}), nil
	case errors.Is(err, folders.ErrForbidden):
		return toolError(codeForbiddenRole, roleDeniedMessage), nil
	case err != nil:
		return Result{}, err
	}
	return success("Asked for comment access to "+string(kind)+" "+id.String(), accessrequests.JSONOf(request)), nil
}

type accessRequestListOut struct {
	accessrequests.ListJSON
	leftOutOut
}

func (r *Registry) listAccessRequests(ctx context.Context, input json.RawMessage) (Result, error) {
	if r.deps.AccessRequests == nil {
		return Result{}, errAccessRequestsNotConfigured
	}
	var in struct{}
	if res, ok := decodeInput(input, &in); !ok {
		return res, nil
	}
	titles, res, ok, err := r.restrictedTitles(ctx, false)
	if err != nil || !ok {
		return res, err
	}
	principal, _ := auth.FromContext(ctx)
	items, err := r.deps.AccessRequests.List(ctx, principal)
	if errors.Is(err, folders.ErrForbidden) {
		return toolError(codeForbiddenRole, roleDeniedMessage), nil
	}
	if err != nil {
		return Result{}, err
	}
	items = keepReadable(titles, items, func(a accessrequests.Request) (string, string) { return a.Folder.ID, a.Object.ID.String() })
	titles.done(ctx)
	return success("Listed "+strconv.Itoa(len(items))+" open access requests", accessRequestListOut{ListJSON: accessrequests.ListJSONOf(items), leftOutOut: titles.report("access request", "access requests")}), nil
}
