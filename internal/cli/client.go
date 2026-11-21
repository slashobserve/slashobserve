package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/forbidden"
)

const (
	requestTimeout = 30 * time.Second
	maxBodyBytes   = 32 << 20
)

var (
	errUnauthorized = errors.New(msgSignIn)
	idPattern       = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

type client struct {
	base      *url.URL
	token     string
	userAgent string
	http      *http.Client
}

const (
	outboundDomainHint = "A key cannot change the allowlist: an Admin adds the domain in the web app under Settings, Outbound access, then run the command again."
	badCursorHint      = "Pass the next_cursor exactly as the previous page printed it, or add --all to read every page."
)

const (
	forbiddenRoleHint   = "This key's service account role does not allow it; an admin can change the role on the API keys page."
	forbiddenAccessHint = "This key cannot see that dashboard or its folder. A folder Admin or an instance Admin decides who sees a folder, but access lists name people and groups, which a key never matches, so use a key of an Admin service account or of the service account that created the folder."
)

type apiError struct {
	status  int
	message string
	code    string
	raw     []byte
	issues  []apiIssue
}

type apiIssue struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

const maxIssueLines = 10

func (e *apiError) Error() string {
	switch {
	case e.message == "":
		return fmt.Sprintf("the server answered HTTP %d", e.status)
	case e.code == "":
		return e.message
	}
	text := fmt.Sprintf("%s (%s)", e.message, e.code)
	for i, issue := range e.issues {
		if i == maxIssueLines {
			text += fmt.Sprintf("\n  and %d more", len(e.issues)-maxIssueLines)
			break
		}
		text += "\n  " + issueLine(issue)
	}
	if hint := e.hint(); hint != "" {
		text += "\n" + hint
	}
	return text
}

func issueLine(issue apiIssue) string {
	if issue.Path == "" {
		return issue.Message
	}
	return issue.Path + ": " + issue.Message
}

func (e *apiError) hint() string {
	switch {
	case e.code == forbidden.Code && e.message == forbidden.RoleMessage:
		return forbiddenRoleHint
	case e.code == forbidden.Code && e.message == forbidden.AccessMessage:
		return forbiddenAccessHint
	case e.code == "outbound_domain_not_allowed":
		return outboundDomainHint
	case e.code == "observation_thread_exists":
		return e.threadExistsHint()
	case e.code == "restricted_content":
		return e.restrictedItemsHint()
	case e.status == http.StatusBadRequest && strings.Contains(e.message, "cursor"):
		return badCursorHint
	}
	return ""
}

func (e *apiError) threadExistsHint() string {
	var body struct {
		ThreadID string `json:"thread_id"`
	}
	if json.Unmarshal(e.raw, &body) != nil || !idPattern.MatchString(body.ThreadID) {
		return "An observation has one discussion: reply to it with observe comments reply <thread-id> <text>."
	}
	return "An observation has one discussion: reply to it with observe comments reply " + body.ThreadID + " <text>."
}

func (a *app) newClient() (*client, error) {
	resolved, err := a.resolve()
	if err != nil {
		return nil, err
	}
	base, err := normalizeServer(resolved.server)
	if err != nil {
		return nil, err
	}
	if resolved.token == "" {
		return nil, errUnauthorized
	}
	return a.clientFor(base, resolved.token)
}

func (a *app) clientFor(base *url.URL, token string) (*client, error) {
	if !validToken(token) {
		return nil, errors.New("the key contains characters a key never has; check it, or run observe login")
	}
	c := a.keylessClient(base)
	c.token = token
	return c, nil
}

func (a *app) keylessClient(base *url.URL) *client {
	return &client{
		base:      base,
		userAgent: "observe/" + a.version,
		http: &http.Client{
			Timeout:       requestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (c *client) withTimeout(d time.Duration) *client {
	clone := *c
	httpClient := *c.http
	httpClient.Timeout = d
	clone.http = &httpClient
	return &clone
}

func parseID(arg string) (string, error) {
	if !idPattern.MatchString(arg) {
		return "", usagef("%q is not a valid id", text(arg))
	}
	return arg, nil
}

func (c *client) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	return c.do(ctx, http.MethodGet, path, query, nil)
}

func (c *client) post(ctx context.Context, path string, payload any) ([]byte, error) {
	return c.send(ctx, http.MethodPost, path, payload)
}

func (c *client) patch(ctx context.Context, path string, payload any) ([]byte, error) {
	return c.send(ctx, http.MethodPatch, path, payload)
}

func (c *client) delete(ctx context.Context, path string) ([]byte, error) {
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

func (c *client) probe(ctx context.Context, path string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base.JoinPath(path).String(), nil)
	if err != nil {
		return 0, nil, fmt.Errorf("build the request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, c.transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return 0, nil, c.transportError(ctx, err)
	}
	return resp.StatusCode, data, nil
}

func (c *client) send(ctx context.Context, method, path string, payload any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode the request: %w", err)
	}
	return c.do(ctx, method, path, nil, data)
}

func (c *client) newRequest(ctx context.Context, method, path string, query url.Values, payload []byte, accept string) (*http.Request, error) {
	endpoint := c.base.JoinPath("api", "v1", path)
	endpoint.RawQuery = query.Encode()
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body) //nolint:gosec,nolintlint // calling the server the user configured is the CLI's purpose
	if err != nil {
		return nil, fmt.Errorf("build the request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func statusError(resp *http.Response, data []byte) error {
	if resp.StatusCode == http.StatusUnauthorized {
		return errUnauthorized
	}
	if resp.StatusCode >= 300 && resp.StatusCode <= 399 {
		return redirectError(resp)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return decodeAPIError(resp.StatusCode, data)
	}
	return nil
}

func (c *client) do(ctx context.Context, method, path string, query url.Values, payload []byte) ([]byte, error) {
	req, err := c.newRequest(ctx, method, path, query, payload, "application/json")
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req) //nolint:gosec,nolintlint // calling the server the user configured is the CLI's purpose
	if err != nil {
		return nil, c.transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, c.transportError(ctx, err)
	}
	if len(data) > maxBodyBytes {
		return nil, errors.New("the server's response is too large")
	}
	if err := statusError(resp, data); err != nil {
		return nil, err
	}
	return data, nil
}

func (c *client) stream(ctx context.Context, path string, query url.Values, lastEventID string) (io.ReadCloser, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, query, nil, "text/event-stream")
	if err != nil {
		return nil, err
	}
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	resp, err := c.open(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (c *client) download(ctx context.Context, path string, query url.Values, accept string) (*http.Response, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, query, nil, accept)
	if err != nil {
		return nil, err
	}
	return c.open(ctx, req)
}

func (c *client) open(ctx context.Context, req *http.Request) (*http.Response, error) {
	open := c.withTimeout(0)
	resp, err := open.http.Do(req)
	if err != nil {
		return nil, open.transportError(ctx, err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return resp, nil
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, open.transportError(ctx, err)
	}
	return nil, statusError(resp, data)
}

func redirectError(resp *http.Response) error {
	target, err := resp.Location()
	if err != nil || target.Host == "" {
		return errors.New("the server answered with a redirect; observe does not follow redirects, check the server URL")
	}
	return fmt.Errorf("the server answered with a redirect to %s; observe does not follow redirects, check the server URL", text(target.Host))
}

func decodeAPIError(status int, data []byte) error {
	var body struct {
		Error  string     `json:"error"`
		Code   string     `json:"code"`
		Issues []apiIssue `json:"issues"`
	}
	if json.Unmarshal(data, &body) != nil {
		return &apiError{status: status, raw: data}
	}
	for i := range body.Issues {
		body.Issues[i].Path, body.Issues[i].Message = text(body.Issues[i].Path), text(body.Issues[i].Message)
	}
	return &apiError{status: status, message: text(body.Error), code: text(body.Code), raw: data, issues: body.Issues}
}

type transportFailure struct{ error }

func (e transportFailure) Unwrap() error { return e.error }

func isTransient(err error) bool {
	var failure transportFailure
	var api *apiError
	return errors.As(err, &failure) || (errors.As(err, &api) && api.status >= 500)
}

func (c *client) transportError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return errInterrupted
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return transportFailure{fmt.Errorf("the server at %s did not answer within %s", c.base.Host, c.http.Timeout)}
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	return transportFailure{fmt.Errorf("cannot reach the server at %s: %w", c.base.Host, err)}
}

func decode[T any](data []byte) (T, error) {
	var value T
	if err := json.Unmarshal(data, &value); err != nil {
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			return value, errors.New("the address answered with something that is not JSON, so it is probably not a SlashObserve server: check --server or OBSERVE_SERVER, which is the address you open in the browser, with no extra path")
		}
		return value, errors.New("the server sent a response the CLI cannot read; check that the CLI and the server versions match")
	}
	return value, nil
}

func (e *apiError) restrictedItemsHint() string {
	var body struct {
		Items []struct {
			Kind        string `json:"kind"`
			ID          string `json:"id"`
			Title       string `json:"title"`
			FolderTitle string `json:"folder_title"`
		} `json:"items"`
		Hidden int `json:"hidden"`
	}
	if json.Unmarshal(e.raw, &body) != nil || len(body.Items) == 0 && body.Hidden == 0 {
		return ""
	}
	lines := make([]string, 0, len(body.Items)+1)
	for _, it := range body.Items {
		lines = append(lines, fmt.Sprintf("  %s %s %q in folder %q", strings.ReplaceAll(it.Kind, "_", " "), it.ID, it.Title, it.FolderTitle))
	}
	if body.Hidden > 0 {
		lines = append(lines, fmt.Sprintf("  and %d in people's My Dashboards", body.Hidden))
	}
	return strings.Join(lines, "\n")
}
