package scheduler

type Metrics interface {
	JobRegistered(job string, outcomes []string)
	JobStarted(job string)
	JobFinished(job, outcome string)
}
