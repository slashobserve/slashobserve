package synth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAddr     = ":8080"
	shutdownTimeout = 5 * time.Second

	KindCounter = "counter"
	KindGauge   = "gauge"

	counterPerSecond = 1.0
	gaugeValue       = 1.0
)

var (
	metricName = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	labelName  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

type Metric struct {
	Name   string              `json:"name"`
	Kind   string              `json:"kind"`
	Labels []map[string]string `json:"labels"`
}

func Load(path string) ([]Metric, error) {
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

func Parse(raw []byte) ([]Metric, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var metrics []Metric
	if err := decoder.Decode(&metrics); err != nil {
		return nil, fmt.Errorf("parse series: %w", err)
	}
	if decoder.More() {
		return nil, errors.New("parse series: trailing data after the list")
	}
	seen := make(map[string]bool, len(metrics))
	for i, m := range metrics {
		if err := m.validate(); err != nil {
			return nil, fmt.Errorf("series %d: %w", i, err)
		}
		if seen[m.Name] {
			return nil, fmt.Errorf("series %d: duplicate name %q", i, m.Name)
		}
		seen[m.Name] = true
	}
	return metrics, nil
}

func (m Metric) validate() error {
	if !metricName.MatchString(m.Name) {
		return fmt.Errorf("invalid metric name %q", m.Name)
	}
	if m.Kind != KindCounter && m.Kind != KindGauge {
		return fmt.Errorf("%s: kind must be %s or %s, got %q", m.Name, KindCounter, KindGauge, m.Kind)
	}
	for _, labels := range m.Labels {
		for name := range labels {
			if !labelName.MatchString(name) || strings.HasPrefix(name, "__") {
				return fmt.Errorf("%s: invalid label name %q", m.Name, name)
			}
		}
	}
	return nil
}

func Handler(metrics []Metric, start time.Time, now func() time.Time) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write(render(metrics, now().Sub(start)))
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

func render(metrics []Metric, elapsed time.Duration) []byte {
	if elapsed < 0 {
		elapsed = 0
	}
	var out bytes.Buffer
	for _, m := range metrics {
		value := gaugeValue
		if m.Kind == KindCounter {
			value = counterPerSecond * elapsed.Seconds()
		}
		fmt.Fprintf(&out, "# TYPE %s %s\n", m.Name, m.Kind)
		labelSets := m.Labels
		if len(labelSets) == 0 {
			labelSets = []map[string]string{nil}
		}
		for _, labels := range labelSets {
			out.WriteString(m.Name)
			out.WriteString(formatLabels(labels))
			out.WriteByte(' ')
			out.WriteString(strconv.FormatFloat(value, 'g', -1, 64))
			out.WriteByte('\n')
		}
	}
	return out.Bytes()
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	names := make([]string, 0, len(labels))
	for name := range labels {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteByte('{')
	for i, name := range names {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(name)
		b.WriteString(`="`)
		b.WriteString(labelValueEscaper.Replace(labels[name]))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

var labelValueEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`, `"`, `\"`)

func Run(ctx context.Context, lookup func(string) (string, bool)) error {
	path, _ := lookup("SYNTH_FILE")
	if path = strings.TrimSpace(path); path == "" {
		return errors.New("SYNTH_FILE is required")
	}
	metrics, err := Load(path)
	if err != nil {
		return err
	}
	addr, _ := lookup("SHOP_ADDR")
	if addr = strings.TrimSpace(addr); addr == "" {
		addr = defaultAddr
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           Handler(metrics, time.Now(), time.Now),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(listener) }()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
