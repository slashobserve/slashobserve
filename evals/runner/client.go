package main

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
	"strings"
	"time"
)

const (
	callTimeout     = 30 * time.Second
	pollInterval    = time.Second
	createBackoff   = 2 * time.Second
	maxCreateWait   = 15 * time.Second
	drainGrace      = 3 * time.Minute
	maxBodyBytes    = 16 << 20
	maxPerPrincipal = 3
)

type client struct {
	base  string
	token string
	http  *http.Client

	poll    time.Duration
	backoff time.Duration
	grace   time.Duration
}

type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("http %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("http %d %s: %s", e.Status, e.Code, e.Message)
}

type Usage struct {
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	CacheReadTokens  int64  `json:"cache_read_tokens"`
	CacheWriteTokens int64  `json:"cache_write_tokens"`
	ModelCalls       int    `json:"model_calls"`
	RepairRounds     int    `json:"repair_rounds"`
}

type AgentRequest struct {
	ID            string     `json:"id"`
	Status        string     `json:"status"`
	DashboardID   *string    `json:"dashboard_id"`
	ResultVersion *int       `json:"result_version"`
	Error         *string    `json:"error"`
	CreatedAt     time.Time  `json:"created_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	Usage         *Usage     `json:"usage"`
}

func (r AgentRequest) finished() bool {
	return r.Status == "succeeded" || r.Status == "failed"
}

type AgentInfo struct {
	Available bool    `json:"available"`
	Provider  *string `json:"provider"`
	Model     *string `json:"model"`
}

func newClient(base, token string) (*client, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if base == "" {
		return nil, errors.New("--server (or OBSERVE_SERVER) is required")
	}
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("--server (or OBSERVE_SERVER) must be an http or https URL")
	}
	if token == "" {
		return nil, errors.New("--token (or OBSERVE_TOKEN) is required")
	}
	return &client{
		base:  u.String(),
		token: token,
		http:  &http.Client{Timeout: callTimeout},

		poll:    pollInterval,
		backoff: createBackoff,
		grace:   drainGrace,
	}, nil
}

func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/api/v1"+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return decodeAPIError(resp.StatusCode, raw)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func decodeAPIError(status int, raw []byte) error {
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if json.Unmarshal(raw, &body) != nil || body.Error == "" {
		return &apiError{Status: status, Message: http.StatusText(status)}
	}
	return &apiError{Status: status, Code: body.Code, Message: body.Error}
}

func (c *client) agent(ctx context.Context) (AgentInfo, error) {
	var info AgentInfo
	err := c.do(ctx, http.MethodGet, "/agent", nil, &info)
	return info, err
}

func (c *client) createObserve(ctx context.Context, prompt string) (AgentRequest, error) {
	var env struct {
		Request AgentRequest `json:"request"`
	}
	err := c.do(ctx, http.MethodPost, "/observe", map[string]string{"prompt": prompt}, &env)
	return env.Request, err
}

func slotsFull(err error) bool {
	var api *apiError
	return errors.As(err, &api) && api.Status == http.StatusTooManyRequests && (api.Code == "too_many_requests" || api.Code == "agent_busy")
}

func (c *client) createObserveWhenFree(ctx context.Context, prompt string) (AgentRequest, error) {
	wait := c.backoff
	for {
		req, err := c.createObserve(ctx, prompt)
		if !slotsFull(err) {
			return req, err
		}
		select {
		case <-ctx.Done():
			return req, err
		case <-time.After(wait):
		}
		wait = min(wait*2, maxCreateWait)
	}
}

func (c *client) getObserve(ctx context.Context, id string) (AgentRequest, error) {
	var env struct {
		Request AgentRequest `json:"request"`
	}
	err := c.do(ctx, http.MethodGet, "/observe/"+url.PathEscape(id), nil, &env)
	return env.Request, err
}

func transient(err error) bool {
	var api *apiError
	if errors.As(err, &api) {
		return api.Status >= 500
	}
	var netErr net.Error
	var urlErr *url.Error
	return errors.As(err, &netErr) || errors.As(err, &urlErr) || errors.Is(err, io.ErrUnexpectedEOF)
}

func (c *client) waitObserve(ctx context.Context, last AgentRequest) (AgentRequest, error) {
	ticker := time.NewTicker(c.poll)
	defer ticker.Stop()
	for {
		req, err := c.getObserve(ctx, last.ID)
		switch {
		case err == nil:
			last = req
			if last.finished() {
				return last, nil
			}
		case ctx.Err() != nil || !transient(err):
			return last, err
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *client) drain(ctx context.Context, last AgentRequest) {
	ctx, cancel := context.WithTimeout(ctx, c.grace)
	defer cancel()
	_, _ = c.waitObserve(ctx, last)
}

func (c *client) getVersionSpec(ctx context.Context, dashboardID string, version int) ([]byte, error) {
	var env struct {
		Version struct {
			Spec json.RawMessage `json:"spec"`
		} `json:"version"`
	}
	path := fmt.Sprintf("/dashboards/%s/versions/%d", url.PathEscape(dashboardID), version)
	if err := c.do(ctx, http.MethodGet, path, nil, &env); err != nil {
		return nil, err
	}
	return env.Version.Spec, nil
}

type Field struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Values []any  `json:"values"`
}

type Frame struct {
	Fields []Field `json:"fields"`
}

type PanelData struct {
	Frames []Frame `json:"frames"`
}

func (c *client) getPanelData(ctx context.Context, dashboardID string, version int, panelID string) (PanelData, error) {
	var data PanelData
	path := fmt.Sprintf("/dashboards/%s/versions/%d/panels/%s/data", url.PathEscape(dashboardID), version, url.PathEscape(panelID))
	err := c.do(ctx, http.MethodGet, path, nil, &data)
	return data, err
}
