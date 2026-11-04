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
	"slices"
	"strings"
	"sync"
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

	serverRunLimit     = 120 * time.Minute
	serverSettleMargin = 5 * time.Minute
	defaultTimeout     = serverRunLimit + serverSettleMargin

	runnerTimeout = "runner_timeout"
)

type client struct {
	base  string
	token string
	http  *http.Client

	poll    time.Duration
	backoff time.Duration
	grace   time.Duration

	// the server grants one import at a time per key, and the agent's import tool takes the same slot as the runner's seeding
	importMu sync.Mutex
}

func (c *client) holdImportSlot() (release func()) {
	c.importMu.Lock()
	return c.importMu.Unlock
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

	ToolCalls           int   `json:"tool_calls"`
	FailedCalls         int   `json:"failed_calls"`
	Retries             int   `json:"retries"`
	ModelMS             int64 `json:"model_ms"`
	ToolMS              int64 `json:"tool_ms"`
	MaxSaveOutputTokens int64 `json:"max_save_output_tokens"`

	BuildSaves       int `json:"build_saves"`
	Reseeds          int `json:"reseeds"`
	Renewals         int `json:"renewals"`
	Compactions      int `json:"compactions"`
	Nudges           int `json:"nudges"`
	SaveNowRefusals  int `json:"save_now_refusals"`
	StalledTurns     int `json:"stalled_turns"`
	OnePanelRefusals int `json:"one_panel_refusals"`
}

type Step struct {
	At     time.Time `json:"at"`
	Phase  string    `json:"phase"`
	Tool   string    `json:"tool"`
	Status string    `json:"status"`
}

type AgentRequest struct {
	ID            string     `json:"id"`
	Status        string     `json:"status"`
	DashboardID   *string    `json:"dashboard_id"`
	ResultVersion *int       `json:"result_version"`
	Error         *string    `json:"error"`
	FailureCode   *string    `json:"failure_code"`
	CreatedAt     time.Time  `json:"created_at"`
	FinishedAt    *time.Time `json:"finished_at"`
	Usage         *Usage     `json:"usage"`
	Steps         []Step     `json:"steps"`

	FirstSaveAt *time.Time `json:"-"`
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
		return c.withoutToken(decodeAPIError(resp.StatusCode, raw))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (c *client) withoutToken(err error) error {
	var api *apiError
	if c.token != "" && errors.As(err, &api) {
		api.Message = strings.ReplaceAll(api.Message, c.token, "[redacted]")
	}
	return err
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
	return errors.As(err, &api) && api.Status == http.StatusTooManyRequests && (api.Code == "too_many_requests" || api.Code == "agent_busy" || api.Code == "too_many_revisions")
}

func importsBusy(err error) bool {
	var api *apiError
	return errors.As(err, &api) && api.Status == http.StatusTooManyRequests && api.Code == "too_many_imports"
}

func (c *client) whenFree(ctx context.Context, busy func(error) bool, attempt func() error) error {
	wait := c.backoff
	for {
		err := attempt()
		if !busy(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(wait):
		}
		wait = min(wait*2, maxCreateWait)
	}
}

func (c *client) createObserveWhenFree(ctx context.Context, prompt string) (AgentRequest, error) {
	var req AgentRequest
	err := c.whenFree(ctx, slotsFull, func() (err error) {
		req, err = c.createObserve(ctx, prompt)
		return err
	})
	return req, err
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
			seen := last.FirstSaveAt
			last = req
			last.FirstSaveAt = earliestSave(req.Steps, seen)
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

func (c *client) drain(ctx context.Context, last AgentRequest) AgentRequest {
	ctx, cancel := context.WithTimeout(ctx, c.grace)
	defer cancel()
	last, _ = c.waitObserve(ctx, last)
	return last
}

type UnverifiedPanel struct {
	PanelID string `json:"panel_id"`
}

type UnverifiedVariable struct {
	Name string `json:"name"`
}

type Unverified struct {
	Panels    []UnverifiedPanel    `json:"panels"`
	Variables []UnverifiedVariable `json:"variables"`
}

func (u Unverified) hasPanel(id string) bool {
	return slices.ContainsFunc(u.Panels, func(p UnverifiedPanel) bool { return p.PanelID == id })
}

func (u Unverified) variableNames() []string {
	var names []string
	for _, v := range u.Variables {
		names = append(names, v.Name)
	}
	return names
}

type Version struct {
	Version    int             `json:"version"`
	Spec       json.RawMessage `json:"spec"`
	Unverified Unverified      `json:"unverified"`
}

func (c *client) getVersion(ctx context.Context, dashboardID string, version int) (Version, error) {
	var env struct {
		Version Version `json:"version"`
	}
	path := fmt.Sprintf("/dashboards/%s/versions/%d", url.PathEscape(dashboardID), version)
	err := c.do(ctx, http.MethodGet, path, nil, &env)
	return env.Version, err
}

func (c *client) getVersionSpec(ctx context.Context, dashboardID string, version int) ([]byte, error) {
	v, err := c.getVersion(ctx, dashboardID, version)
	if err != nil {
		return nil, err
	}
	return v.Spec, nil
}

type Imported struct {
	DashboardID string
	ImportID    string
	Version     int
	Spec        json.RawMessage
	Unverified  Unverified
}

func (c *client) createImport(ctx context.Context, dashboardJSON []byte, mapping map[string]string) (Imported, error) {
	body := map[string]any{"json": string(dashboardJSON)}
	if len(mapping) > 0 {
		body["mapping"] = mapping
	}
	var env struct {
		Dashboard struct {
			ID string `json:"id"`
		} `json:"dashboard"`
		Version Version `json:"version"`
		Import  struct {
			ID string `json:"id"`
		} `json:"import"`
	}
	c.importMu.Lock()
	defer c.importMu.Unlock()
	err := c.whenFree(ctx, importsBusy, func() error {
		return c.do(ctx, http.MethodPost, "/imports", body, &env)
	})
	if err != nil {
		return Imported{}, err
	}
	return Imported{DashboardID: env.Dashboard.ID, ImportID: env.Import.ID, Version: env.Version.Version, Spec: env.Version.Spec, Unverified: env.Version.Unverified}, nil
}

type ImportRecord struct {
	ID          string `json:"id"`
	DashboardID string `json:"dashboard_id"`
	Version     int    `json:"version"`
	Title       string `json:"title"`
}

func (c *client) dashboardImport(ctx context.Context, dashboardID string) (ImportRecord, error) {
	var record ImportRecord
	err := c.do(ctx, http.MethodGet, "/dashboards/"+url.PathEscape(dashboardID)+"/import", nil, &record)
	return record, err
}

func (c *client) createRevision(ctx context.Context, dashboardID string, baseVersion int, panelID, prompt string) (AgentRequest, error) {
	body := map[string]any{"prompt": prompt, "base_version": baseVersion}
	if panelID != "" {
		body["panel_id"] = panelID
	}
	var env struct {
		Revision AgentRequest `json:"revision"`
	}
	err := c.whenFree(ctx, slotsFull, func() error {
		return c.do(ctx, http.MethodPost, "/dashboards/"+url.PathEscape(dashboardID)+"/revisions", body, &env)
	})
	return env.Revision, err
}

type Suggestion struct {
	ID          string          `json:"id"`
	DashboardID string          `json:"dashboard_id"`
	PanelID     string          `json:"panel_id"`
	BaseVersion int             `json:"base_version"`
	Status      string          `json:"status"`
	Panel       json.RawMessage `json:"panel"`
	Error       *string         `json:"error"`
	FailureCode *string         `json:"failure_code"`
	Usage       *Usage          `json:"usage"`
}

func (s Suggestion) finished() bool {
	return s.Status == "succeeded" || s.Status == "failed"
}

func (c *client) createSuggestion(ctx context.Context, dashboardID string, version int, panelID, prompt string) (Suggestion, error) {
	body := map[string]any{"version": version, "panel_id": panelID, "prompt": prompt}
	var env struct {
		Suggestion Suggestion `json:"suggestion"`
	}
	err := c.whenFree(ctx, slotsFull, func() error {
		return c.do(ctx, http.MethodPost, "/dashboards/"+url.PathEscape(dashboardID)+"/suggestions", body, &env)
	})
	return env.Suggestion, err
}

func (c *client) getSuggestion(ctx context.Context, dashboardID, suggestionID string) (Suggestion, error) {
	var env struct {
		Suggestion Suggestion `json:"suggestion"`
	}
	path := "/dashboards/" + url.PathEscape(dashboardID) + "/suggestions/" + url.PathEscape(suggestionID)
	err := c.do(ctx, http.MethodGet, path, nil, &env)
	return env.Suggestion, err
}

func (c *client) datasourceTypes(ctx context.Context) (map[string]string, error) {
	var env struct {
		Datasources []struct {
			UID  string `json:"uid"`
			Type string `json:"type"`
		} `json:"datasources"`
	}
	if err := c.do(ctx, http.MethodGet, "/datasources", nil, &env); err != nil {
		return nil, err
	}
	types := make(map[string]string, len(env.Datasources))
	for _, d := range env.Datasources {
		types[d.UID] = d.Type
	}
	return types, nil
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

func (c *client) getVariableValues(ctx context.Context, dashboardID string, version int, name string) ([]string, error) {
	var body struct {
		Values []string `json:"values"`
	}
	path := fmt.Sprintf("/dashboards/%s/versions/%d/variables/%s/values", url.PathEscape(dashboardID), version, url.PathEscape(name))
	err := c.do(ctx, http.MethodGet, path, nil, &body)
	return body.Values, err
}
