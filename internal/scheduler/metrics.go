package scheduler

type Metrics interface {
	JobRegistered(job string, outcomes []string)
	JobStarted(job string)
	JobFinished(job, outcome string)
}

type NopMetrics struct{}

func (NopMetrics) JobRegistered(string, []string) {}

func (NopMetrics) JobStarted(string) {}

func (NopMetrics) JobFinished(string, string) {}
