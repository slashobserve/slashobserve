package grafana

import (
	"fmt"
	"math"
	"slices"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxGroupFields     = 20
	maxPredictions     = 1000
	maxDegree          = 5
	defaultDegree      = 2
	defaultFirstName   = "Field"
	defaultRestName    = "Value"
	groupByOperation   = "groupby"
	aggregateOperation = "aggregate"

	uniqueValuesReducer = "uniqueValues"
)

var emptyFills = []string{"empty", "zero", "true", "false"}

func (b *panelBuild) groupFields(v any, path string) ([]string, []spec.Aggregation) {
	fields, _ := asObject(v)
	var by []string
	var aggregate []spec.Aggregation
	for _, name := range sortedKeys(fields) {
		item, _ := asObject(fields[name])
		itemPath := path + "." + name
		b.reportOtherKeys(item, []string{"operation", "aggregations"}, itemPath)
		if !validLength(name, maxNameMatch) {
			b.partial(itemPath, "")
			continue
		}
		switch asString(item["operation"]) {
		case groupByOperation:
			if len(by) >= maxGroupFields {
				b.partial(itemPath, fmt.Sprintf("A transformation groups by at most %d fields.", maxGroupFields))
				continue
			}
			by = append(by, name)
		case aggregateOperation:
			reducers := acceptedReducers(b, item["aggregations"], itemPath+".aggregations", isAggregateReducer)
			switch {
			case len(reducers) == 0:
			case len(aggregate) >= maxGroupFields:
				b.partial(itemPath, fmt.Sprintf("A transformation aggregates at most %d fields.", maxGroupFields))
			default:
				aggregate = append(aggregate, spec.Aggregation{Field: name, Reducers: reducers})
			}
		}
	}
	return by, aggregate
}

func groupByTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	if fields, _ := asObject(opts["fields"]); len(fields) == 0 {
		return spec.Transform{}, false
	}
	by, aggregate := b.groupFields(opts["fields"], path+".options.fields")
	return spec.Transform{Type: "groupBy", By: by, Aggregate: aggregate}, true
}

func groupToNestedTableTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	hidden := false
	if shown, set := opts["showSubframeHeaders"].(bool); set && !shown {
		hidden = true
	}
	if fields, _ := asObject(opts["fields"]); len(fields) == 0 {
		if hidden {
			b.partial(path+".options.showSubframeHeaders", "")
		}
		return spec.Transform{}, false
	}
	by, aggregate := b.groupFields(opts["fields"], path+".options.fields")
	return spec.Transform{Type: "groupToNestedTable", By: by, Aggregate: aggregate, HideHeaders: hidden}, true
}

func (b *panelBuild) optionalName(opts map[string]any, key, path string) string {
	if !present(opts[key]) {
		return ""
	}
	name, ok := nameOption(opts, key)
	if !ok {
		b.partial(path+".options."+key, "")
	}
	return name
}

func (b *panelBuild) emptyFill(opts map[string]any, key, path string) string {
	switch fill := asString(opts[key]); {
	case fill == "" || fill == "null":
		return ""
	case slices.Contains(emptyFills, fill):
		return fill
	default:
		b.partial(path+".options."+key, "")
		return ""
	}
}

func groupingToMatrixTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	return spec.Transform{
		Type:   "groupingToMatrix",
		Column: b.optionalName(opts, "columnField", path),
		Row:    b.optionalName(opts, "rowField", path),
		Value:  b.optionalName(opts, "valueField", path),
		Empty:  b.emptyFill(opts, "emptyValue", path),
	}, true
}

func (b *panelBuild) boundedInt(opts map[string]any, key string, low, high int, path string) (int, bool) {
	if !present(opts[key]) {
		return 0, false
	}
	n, ok := numberValue(opts[key])
	if !ok || n != math.Trunc(n) || n < float64(low) || n > float64(high) {
		b.partial(path+".options."+key, "")
		return 0, false
	}
	return int(n), true
}

func histogramTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "histogram", Combine: truthy(opts["combine"])}
	t.BucketCount, _ = b.boundedInt(opts, "bucketCount", 1, maxBucketCount, path)
	if present(opts["bucketSize"]) {
		if size, ok := numberValue(opts["bucketSize"]); ok && size > 0 {
			t.BucketSize = size
		} else {
			b.partial(path+".options.bucketSize", "")
		}
	}
	if present(opts["bucketOffset"]) {
		if offset, ok := numberValue(opts["bucketOffset"]); ok {
			t.BucketOffset = offset
		} else {
			b.partial(path+".options.bucketOffset", "")
		}
	}
	return t, true
}

func regressionTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "regression"}
	switch model := asString(opts["modelType"]); model {
	case "", "linear":
	case "polynomial":
		t.Model = model
		if degree, ok := b.boundedInt(opts, "degree", 1, maxDegree, path); ok && degree != defaultDegree {
			t.Degree = degree
		}
	default:
		return b.dropTransform(path + ".options.modelType")
	}
	t.X = b.optionalName(opts, "xFieldName", path)
	t.Y = b.optionalName(opts, "yFieldName", path)
	t.Predictions, _ = b.boundedInt(opts, "predictionCount", 1, maxPredictions, path)
	return t, true
}

func transposeTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{
		Type:      "transpose",
		FirstName: b.optionalName(opts, "firstFieldName", path),
		RestName:  b.optionalName(opts, "restFieldsName", path),
		Empty:     b.emptyFill(opts, "emptyValue", path),
	}
	if t.FirstName == defaultFirstName {
		t.FirstName = ""
	}
	if t.RestName == defaultRestName {
		t.RestName = ""
	}
	return t, true
}

var (
	bucketKeys      = []string{"mode", "value", "scale"}
	bucketScaleKeys = []string{"type", "log"}
	bucketScales    = []string{"linear", "log", "symlog"}
	bucketModes     = []string{"size", "count"}
)

func (b *panelBuild) transformBuckets(opts map[string]any, key string, size func(any) (float64, bool), path string) (*spec.Buckets, map[string]any) {
	item, ok := asObject(opts[key])
	if !ok {
		if present(opts[key]) {
			b.partial(path+".options."+key, "")
		}
		return nil, nil
	}
	itemPath := path + ".options." + key
	b.reportOtherKeys(item, bucketKeys, itemPath)
	if mode := asString(item["mode"]); present(item["mode"]) && !slices.Contains(bucketModes, mode) {
		b.partial(itemPath+".mode", "")
	}
	scale, _ := asObject(item["scale"])
	b.reportOtherKeys(scale, bucketScaleKeys, itemPath+".scale")
	if kind := asString(scale["type"]); present(scale["type"]) && !slices.Contains(bucketScales, kind) {
		b.partial(itemPath+".scale.type", "")
	}
	return b.heatmapBuckets(itemPath, size), scale
}

func heatmapTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "heatmap", KeepOriginalData: truthy(opts["keepOriginalData"])}
	var xScale, yScale map[string]any
	t.XBuckets, xScale = b.transformBuckets(opts, "xBuckets", xBucketSize, path)
	t.YBuckets, yScale = b.transformBuckets(opts, "yBuckets", yBucketSize, path)
	if present(xScale) {
		b.heatmapXBucketScale(path + ".options.xBuckets.scale")
	}
	if present(yScale) {
		scale, base, written := b.heatmapRowScale(path + ".options.yBuckets.scale")
		if scale != "linear" {
			t.YScale = scale
		}
		if written {
			t.YLogBase = base
		}
	}
	return t, true
}
