package elasticsearch

import (
	"context"
	"encoding/json"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/logvolume"
)

const (
	volumeAgg       = "volume"
	levelAgg        = "level"
	maxLevelBuckets = 20
)

var _ datasource.LogVolumeSource = (*Datasource)(nil)

func (d *Datasource) LogVolume(ctx context.Context, req datasource.VolumeRequest) (*logvolume.Histogram, error) {
	if err := req.Range.Validate(); err != nil {
		return nil, err
	}
	if req.Step < time.Second || req.Step%time.Second != 0 {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "step must be a whole number of seconds")
	}
	lim := d.opts.Limits.Tighten(req.Query.Limits)
	ctx, cancel := bound(ctx, lim.Timeout)
	defer cancel()
	prep, err := d.prepare(ctx, req.Query, req.Range, req.Step, lim)
	if err != nil {
		return nil, err
	}
	if prep.user.aggs() != nil {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "the volume needs a KQL, Lucene or filter-only query body")
	}
	roles, err := prep.mapping.roles(d.opts.Fields)
	if err != nil {
		return nil, err
	}
	step, err := histogramStep(req.Range, req.Step, lim)
	if err != nil {
		return nil, err
	}
	raw, err := d.search(ctx, volumeBody(prep, d.opts.TimeField, req.Range, roles.Level, step, lim))
	if err != nil {
		return nil, err
	}
	return foldVolume(raw, req)
}

func volumeBody(prep prepared, timeField string, r datasource.TimeRange, level string, step time.Duration, lim datasource.Limits) map[string]any {
	counted := prep.user
	counted.dsl = nil
	body := buildBody(counted, timeField, r, prep.adhoc, plan{kind: searchCount, step: step, timeout: lim.Timeout})
	histogram := body["aggs"].(map[string]any)[countAgg].(map[string]any)
	if terms, ok := levelTerms(prep.mapping, level); ok {
		histogram["aggs"] = map[string]any{levelAgg: terms}
	}
	body["aggs"] = map[string]any{volumeAgg: histogram}
	if runtime, ok := prep.user.dsl["runtime_mappings"]; ok {
		body["runtime_mappings"] = runtime
	}
	return body
}

func levelTerms(m *Mapping, level string) (map[string]any, bool) {
	if level == "" {
		return nil, false
	}
	field, err := m.termsField(level)
	if err != nil {
		return nil, false
	}
	return map[string]any{"terms": map[string]any{"field": field, "size": maxLevelBuckets}}, true
}

type volumeResponse struct {
	Buckets []struct {
		Key      float64 `json:"key"`
		DocCount int64   `json:"doc_count"`
		Level    struct {
			Buckets []struct {
				Key      json.RawMessage `json:"key"`
				DocCount int64           `json:"doc_count"`
			} `json:"buckets"`
		} `json:"level"`
	} `json:"buckets"`
}

func foldVolume(raw []byte, req datasource.VolumeRequest) (*logvolume.Histogram, error) {
	resp, err := parseSearch(raw)
	if err != nil {
		return nil, err
	}
	if resp.TimedOut {
		return nil, datasource.Errorf(datasource.ErrTimeout, "the search timed out before every shard answered")
	}
	if resp.Shards.Failed > 0 {
		return nil, datasource.Errorf(datasource.ErrUnavailable, "some shards failed, so the counts would be incomplete")
	}
	hist := logvolume.New(req.Range.From, req.Step, req.Buckets())
	tree, ok := resp.Aggregations[volumeAgg]
	if !ok {
		return hist, nil
	}
	var volume volumeResponse
	if err := json.Unmarshal(tree, &volume); err != nil {
		return nil, unreadable()
	}
	for _, bucket := range volume.Buckets {
		start := time.UnixMilli(int64(bucket.Key)).UTC()
		leveled := int64(0)
		for _, level := range bucket.Level.Buckets {
			hist.Add(start, logvolume.KindOf(levelWord(level.Key)), level.DocCount)
			leveled += level.DocCount
		}
		if rest := bucket.DocCount - leveled; rest > 0 {
			hist.Add(start, logvolume.Other, rest)
		}
	}
	return hist, nil
}

func levelWord(key json.RawMessage) string {
	var word string
	if json.Unmarshal(key, &word) == nil {
		return word
	}
	return string(key)
}
