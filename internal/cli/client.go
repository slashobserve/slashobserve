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
	"time"
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

type apiError struct {
	status  int
	message string
	code    string
}

func (e *apiError) Error() string {
	switch {
	case e.message == "":
		return fmt.Sprintf("the server answered HTTP %d", e.status)
	case e.code == "":
		return e.message
	}
	return fmt.Sprintf("%s (%s)", e.message, e.code)
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
	return &client{
		base:      base,
		token:     token,
		userAgent: "observe/" + a.version,
		http: &http.Client{
			Timeout:       requestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
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
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode the request: %w", err)
	}
	return c.do(ctx, http.MethodPost, path, nil, data)
}

func (c *client) do(ctx context.Context, method, path string, query url.Values, payload []byte) ([]byte, error) {
	endpoint := c.base.JoinPath("api", "v1", path)
	endpoint.RawQuery = query.Encode()
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body) //nolint:gosec // calling the server the user configured is the CLI's purpose
	if err != nil {
		return nil, fmt.Errorf("build the request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req) //nolint:gosec // calling the server the user configured is the CLI's purpose
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
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errUnauthorized
	}
	if resp.StatusCode >= 300 && resp.StatusCode <= 399 {
		return nil, redirectError(resp)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, decodeAPIError(resp.StatusCode, data)
	}
	return data, nil
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
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if json.Unmarshal(data, &body) != nil {
		return &apiError{status: status}
	}
	return &apiError{status: status, message: text(body.Error), code: text(body.Code)}
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
		return transportFailure{fmt.Errorf("the server at %s did not answer within %s", c.base.Host, requestTimeout)}
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
		return value, errors.New("the server sent a response the CLI cannot read; check that the CLI and the server versions match")
	}
	return value, nil
}
