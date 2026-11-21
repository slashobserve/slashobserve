package export

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	sendTimeout     = 30 * time.Second
	maxResponseBody = 64 << 10
)

type Receipt struct {
	Rejected int64
	Status   int
}

type rejectedCount int64

func (c *rejectedCount) UnmarshalJSON(raw []byte) error {
	n, err := strconv.ParseInt(strings.Trim(string(raw), `"`), 10, 64)
	if err != nil {
		return err
	}
	*c = rejectedCount(n)
	return nil
}

type answer struct {
	PartialSuccess struct {
		Rejected rejectedCount `json:"rejectedLogRecords"`
	} `json:"partialSuccess"`
}

func rejectedIn(body []byte) int64 {
	var a answer
	if json.Unmarshal(body, &a) != nil || a.PartialSuccess.Rejected <= 0 {
		return 0
	}
	return int64(a.PartialSuccess.Rejected)
}

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

func (s *Sender) Send(ctx context.Context, body []byte) (Receipt, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.logs, bytes.NewReader(body))
	if err != nil {
		return Receipt{}, err
	}
	for name, value := range s.headers {
		req.Header.Set(name, value.Reveal())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SlashObserve audit export")
	resp, err := s.client.Do(req)
	if err != nil {
		return Receipt{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	answered, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody+1))
	if resp.StatusCode/100 != 2 {
		return Receipt{}, &StatusError{Status: resp.StatusCode, RetryAfter: retryAfter(resp.Header)}
	}
	receipt := Receipt{Status: resp.StatusCode}
	if len(answered) <= maxResponseBody {
		receipt.Rejected = rejectedIn(answered)
	}
	return receipt, nil
}

func retryAfter(header http.Header) time.Duration {
	seconds, err := strconv.ParseUint(header.Get("Retry-After"), 10, 31)
	if err != nil {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
