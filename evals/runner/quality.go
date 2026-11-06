package main

import (
	"fmt"
	"strings"

	"github.com/slashobserve/slashobserve/internal/taste"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxListedQualityFailures = 5

var qualityWeights = map[string]float64{
	taste.Titles:       0.10,
	taste.Units:        0.20,
	taste.Legends:      0.15,
	taste.Descriptions: 0.10,
	taste.Thresholds:   0.10,
	taste.FullLines:    0.15,
	taste.Headline:     0.10,
	taste.RowQuestions: 0.05,
	taste.LonePanels:   0.05,
	taste.Stacking:     0,
	taste.IDs:          0,
}

func scoreQuality(d *spec.Dashboard) (float64, []string) {
	score := 0.0
	var findings []string
	for _, j := range taste.Judge(d) {
		weight := qualityWeights[j.Rule]
		if weight == 0 {
			continue
		}
		score += weight * share(j)
		if len(j.Failed) > 0 {
			findings = append(findings, j.Rule+": "+listFirst(j.Failed, maxListedQualityFailures))
		}
	}
	return score, findings
}

func share(j taste.Judgement) float64 {
	if j.Checked == 0 {
		return 1
	}
	return 1 - float64(len(j.Failed))/float64(j.Checked)
}

func listFirst(items []string, limit int) string {
	if len(items) <= limit {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(items[:limit], ", "), len(items)-limit)
}
