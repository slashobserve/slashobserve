package scheduler

type Metrics interface {
	JobStarted(job string)
	JobFinished(job, outcome string)
}

type NopMetrics struct{}

func (NopMetrics) JobStarted(string) {}

func (NopMetrics) JobFinished(string, string) {}
