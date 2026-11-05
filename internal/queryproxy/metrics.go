package queryproxy

import "time"

const outcomeOK = "ok"

type Metrics interface {
	ProxyQuery(datasourceType, outcome string, took time.Duration)
	ProxyCache(hit bool)
}

type noMetrics struct{}

func (noMetrics) ProxyQuery(string, string, time.Duration) {}
func (noMetrics) ProxyCache(bool)                          {}

func WithMetrics(m Metrics) RunnerOption {
	return func(r *Runner) { r.metrics = m }
}

func (r *Runner) RecordCacheLookup(hit bool) {
	r.metrics.ProxyCache(hit)
}

func queryOutcome(err error) string {
	if err == nil {
		return outcomeOK
	}
	_, code, _ := Classify(err)
	return code
}
