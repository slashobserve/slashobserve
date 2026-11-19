package tools

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const codeWrongZone = "wrong_time_zone"

var utcLabelledTime = regexp.MustCompile(`\b\d{1,2}:\d{2}(?::\d{2})?\s*(?:UTC|GMT)\b(?:[+-]\d)?|\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(?::\d{2}(?:\.\d+)?)?Z\b`)

func ZoneLabel(loc *time.Location, at time.Time) string {
	name, offset := at.In(loc).Zone()
	if name != "" && name[0] != '+' && name[0] != '-' {
		return name
	}
	sign := '+'
	if offset < 0 {
		sign, offset = '-', -offset
	}
	return fmt.Sprintf("UTC%c%02d:%02d", sign, offset/3600, offset%3600/60)
}

func zoneLabels(loc *time.Location, at ...time.Time) []string {
	var labels []string
	for _, t := range at {
		if label := ZoneLabel(loc, t); !slices.Contains(labels, label) {
			labels = append(labels, label)
		}
	}
	return labels
}

func labelsUTCOnly(markdown string) bool {
	for _, m := range utcLabelledTime.FindAllString(markdown, -1) {
		if last := m[len(m)-1]; last < '0' || last > '9' {
			return true
		}
	}
	return false
}

func zoneMismatch(markdown, zone string, at ...time.Time) (Result, bool) {
	loc, err := spec.LoadZone(zone)
	if zone == "" || err != nil || !labelsUTCOnly(markdown) {
		return Result{}, false
	}
	labels := zoneLabels(loc, at...)
	for _, label := range labels {
		if strings.Contains(markdown, label) {
			return Result{}, false
		}
	}
	res := toolError(codeWrongZone, fmt.Sprintf("the finding labels times UTC, but the person reads times in %s: write each time in that zone with its label, such as 14:05 %s", zone, labels[0]))
	res.Step = "Finding gave times in UTC instead of " + zone + "; the agent must convert them"
	return res, true
}
