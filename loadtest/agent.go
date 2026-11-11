package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

const (
	buildPrompt   = "replicas stack build"
	retryEvery    = time.Second
	pollEvery     = time.Second
	queueWaitP95  = 5000
	statusDone    = "succeeded"
	statusFailed  = "failed"
	submitAccepts = 202
)

type buildOutcome struct {
	done     bool
	ok       bool
	attempts int
	failure  string
	message  string
	took     time.Duration
	retries  int
}

type buildRun struct {
	e       *env
	submit  *recorder
	polling *recorder
}

func (b *buildRun) accept(ctx context.Context, key string, out *buildOutcome, deadline time.Time) (string, bool) {
	for time.Now().Before(deadline) && ctx.Err() == nil {
		r := b.e.api.post(ctx, key, "/api/v1/observe", map[string]string{"prompt": buildPrompt})
		if r.status == submitAccepts {
			var created struct {
				Request struct {
					ID string `json:"id"`
				} `json:"request"`
			}
			if json.Unmarshal(r.body, &created) == nil && created.Request.ID != "" {
				b.submit.Observe(r.took)
				return created.Request.ID, true
			}
			out.failure = "the accepted build carries no id"
			return "", false
		}
		if r.status != tooManyRequests {
			track(ctx, r, b.submit)
			out.failure = r.describe()
			return "", false
		}
		b.submit.Fail(r.status, r.code())
		out.retries++
		if !b.e.sleep(ctx, retryEvery+time.Duration(out.retries%5)*50*time.Millisecond) {
			break
		}
	}
	out.failure = "the build was not accepted before the timeout"
	return "", false
}

func (b *buildRun) await(ctx context.Context, key, id string, out *buildOutcome, deadline time.Time) {
	start := time.Now()
	for time.Now().Before(deadline) && b.e.sleep(ctx, pollEvery) {
		r := b.e.api.get(ctx, key, "/api/v1/observe/"+id)
		track(ctx, r, b.polling)
		if !r.ok() {
			continue
		}
		var got struct {
			Request struct {
				Status      string `json:"status"`
				Attempts    int    `json:"attempts"`
				FailureCode string `json:"failure_code"`
				Error       string `json:"error"`
			} `json:"request"`
		}
		if json.Unmarshal(r.body, &got) != nil {
			continue
		}
		if got.Request.Status == statusDone || got.Request.Status == statusFailed {
			out.done = true
			out.ok = got.Request.Status == statusDone
			out.attempts = got.Request.Attempts
			out.failure = got.Request.FailureCode
			out.message = got.Request.Error
			out.took = time.Since(start)
			return
		}
	}
	out.failure = "the build did not finish before the timeout"
}

func (b *buildRun) one(ctx context.Context, i int) buildOutcome {
	key := b.e.editorKey(i)
	deadline := time.Now().Add(b.e.cfg.buildTimeout)
	var out buildOutcome
	id, ok := b.accept(ctx, key, &out, deadline)
	if !ok {
		return out
	}
	b.await(ctx, key, id, &out, deadline)
	return out
}

func runAgent(ctx context.Context, e *env) (result, error) {
	b := &buildRun{e: e, submit: newRecorder(), polling: newRecorder()}
	poolQueries := []gaugeQuery{{key: "pool", name: "slashobserve_db_pool_connections", labels: map[string]string{"state": "acquired"}}}
	startScrape := e.scrapeAll(ctx)
	tracker, stopTracking := trackGauges(ctx, e.scrapers, poolQueries, gaugeEvery)
	var res result
	var sections []section
	for _, n := range e.cfg.builds {
		sec, gates := b.batch(ctx, n)
		sections = append(sections, sec)
		res.gates = append(res.gates, gates...)
	}
	stopTracking()
	endScrape := e.scrapeAll(ctx)
	res.sections = append(res.sections, sections...)
	res.sections = append(res.sections, e.poolSection(tracker, startScrape, endScrape))
	all := merge(b.submit, b.polling)
	res.gates = append(res.gates, outcomeGates("agent builds", all)...)
	return res, nil
}

func (b *buildRun) batch(ctx context.Context, n int) (section, []gate) {
	e := b.e
	before := e.scrapeAll(ctx)
	outcomes := make([]buildOutcome, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outcomes[i] = b.one(ctx, i)
		}()
	}
	wg.Wait()
	after := e.scrapeAll(ctx)

	client := newRecorder()
	errs := newErrorLog()
	succeeded, retried, submitRetries := 0, 0, 0
	for _, o := range outcomes {
		if !o.ok {
			errs.Add("build", o.failureCode(), truncate(redact(o.message)))
		}
		submitRetries += o.retries
		if o.done && o.ok {
			succeeded++
			client.Observe(o.took)
		}
		if o.attempts > 1 {
			retried++
		}
	}
	delta := histogramDelta(before, after, "slashobserve_agent_queue_wait_seconds", map[string]string{"source": "observe"})
	sec := section{Title: fmt.Sprintf("Agent builds, %d at once", n)}
	sec.Errors = errs.Samples()
	sec.Metrics = []metric{
		countMetric("builds submitted", float64(n)),
		countMetric("builds succeeded", float64(succeeded)),
		countMetric("builds that ran more than once", float64(retried)),
		countMetric("submissions answered 429 and retried", float64(submitRetries)),
		secondsMetric("queue wait p50", histogramQuantile(0.5, delta)),
		secondsMetric("queue wait p95", histogramQuantile(0.95, delta)).under(queueWaitP95, "5 s at 8 workers"),
		durationMetric("build time p50", client.Quantile(0.5)),
		durationMetric("build time p95", client.Quantile(0.95)),
		durationMetric("build time max", client.Max()),
	}
	name := fmt.Sprintf("agent builds, %d at once", n)
	return sec, []gate{
		{Name: "every build succeeds in " + name, Pass: succeeded == n, Detail: failureDetail(outcomes, succeeded, n)},
		{Name: "every build runs once in " + name, Pass: retried == 0, Detail: fmt.Sprintf("%d ran more than once", retried)},
	}
}

func (o buildOutcome) failureCode() string {
	if o.failure == "" {
		return "unknown"
	}
	return o.failure
}

func failureDetail(outcomes []buildOutcome, succeeded, n int) string {
	detail := fmt.Sprintf("%d of %d succeeded", succeeded, n)
	for _, o := range outcomes {
		if !o.ok && o.failure != "" {
			return detail + "; first failure: " + o.failure
		}
	}
	return detail
}

func (e *env) poolSection(tracker *gaugeMax, before, after [][]sample) section {
	sec := section{Title: "Database pool during agent builds"}
	for i := range e.scrapers {
		sec.Metrics = append(sec.Metrics, countMetric(fmt.Sprintf("server %d connections acquired, max", i+1), tracker.get(fmt.Sprintf("pool#%d", i))))
		if i < len(after) && after[i] != nil {
			sec.Metrics = append(sec.Metrics, countMetric(fmt.Sprintf("server %d pool size", i+1), sumOf(after[i], "slashobserve_db_pool_max_connections", nil)))
		}
	}
	empty := counterDelta(before, after, "slashobserve_db_pool_empty_acquires_total", nil)
	sec.Metrics = append(sec.Metrics, countMetric("acquires that waited for a connection", empty))
	return sec
}
