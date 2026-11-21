package export

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	sendTimeout     = 30 * time.Second
	maxResponseBody = 64 << 10
)

type StatusError struct {
	Status     int
	RetryAfter time.Duration
}

func (e *StatusError) Error() string { return "unexpected status " + strconv.Itoa(e.Status) }

type Sender struct {
	client  *http.Client
	logs    string
	headers map[string]datasource.Secret
}

func NewSender(endpoint *url.URL, headers map[string]datasource.Secret) *Sender {
	return &Sender{
		client: &http.Client{
			Transport:     datasource.NewHTTPTransport(),
			Timeout:       sendTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		logs:    endpoint.JoinPath("v1", "logs").String(),
		headers: headers,
	}
}

func (s *Sender) Send(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.logs, bytes.NewReader(body))
	if err != nil {
		return err
	}
	for name, value := range s.headers {
		req.Header.Set(name, value.Reveal())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SlashObserve audit export")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBody))
	if resp.StatusCode/100 != 2 {
		return &StatusError{Status: resp.StatusCode, RetryAfter: retryAfter(resp.Header)}
	}
	return nil
}

func retryAfter(header http.Header) time.Duration {
	seconds, err := strconv.ParseUint(header.Get("Retry-After"), 10, 31)
	if err != nil {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
