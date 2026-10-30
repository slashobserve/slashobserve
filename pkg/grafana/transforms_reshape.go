package grafana

import (
	"cmp"
	"fmt"
	"maps"
	"slices"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	maxJoinLabels      = 20
	maxPartitionFields = 10
	maxConfigMappings  = 20
	maxStats           = 26
	defaultReducer     = "lastNotNull"
	defaultFrameLabel  = "frame"
	defaultStatsTime   = "Time"
)

var seriesFormats = map[string]string{"wide": "wide", "multi": "many", "many": "many", "long": "long"}

var configFieldHandlers = map[string]string{
	"unit": "unit", "min": "min", "max": "max", "decimals": "decimals",
	"displayName": "displayName", "noValue": "noValue", "color": "color",
}

var rowFieldHandlers = func() map[string]string {
	handlers := maps.Clone(configFieldHandlers)
	handlers["field.name"] = "name"
	handlers["field.value"] = "value"
	return handlers
}()

func (b *panelBuild) cappedNames(names []string, limit int, path string) []string {
	out := clippedNames(b, names, path)
	if len(out) > limit {
		b.partial(path, fmt.Sprintf("A transformation keeps at most %d names.", limit))
		out = out[:limit]
	}
	return out
}

func labelsToFieldsTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "labelsToFields"}
	switch mode := cmp.Or(asString(opts["mode"]), "columns"); mode {
	case "columns":
	case "rows":
		t.Mode = mode
	default:
		b.partial(path+".options.mode", "")
	}
	t.Include = clippedNames(b, stringList(opts["keepLabels"]), path+".options.keepLabels")
	if label, ok := nameOption(opts, "valueLabel"); ok {
		t.ValueLabel = label
	}
	return t, true
}

func joinByLabelsTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	value, ok := nameOption(opts, "value")
	if !ok {
		return b.dropTransform(path)
	}
	t := spec.Transform{Type: "joinByLabels", Value: value}
	t.On = b.cappedNames(stringList(opts["join"]), maxJoinLabels, path+".options.join")
	return t, true
}

func partitionByValuesTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	fields := b.cappedNames(stringList(opts["fields"]), maxPartitionFields, path+".options.fields")
	if len(fields) == 0 {
		return b.dropTransform(path)
	}
	t := spec.Transform{Type: "partitionByValues", Fields: fields, KeepFields: truthy(opts["keepFields"])}
	naming, _ := asObject(opts["naming"])
	b.reportOtherKeys(naming, []string{"asLabels", "append"}, path+".options.naming")
	asLabels, set := naming["asLabels"].(bool)
	switch appended := truthy(naming["append"]); {
	case !set || asLabels:
		if appended {
			b.partial(path+".options.naming.append", "")
		}
	case appended:
		t.Naming = "append"
	default:
		t.Naming = "name"
	}
	return t, true
}

func prepareTimeSeriesTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	format, known := seriesFormats[cmp.Or(asString(opts["format"]), "wide")]
	if !known {
		b.partial(path+".options.format", "")
		return spec.Transform{}, false
	}
	return spec.Transform{Type: "prepareTimeSeries", Format: format}, true
}

func timeSeriesTableTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "timeSeriesTable"}
	for _, id := range sortedKeys(opts) {
		itemPath := fmt.Sprintf("%s.options.%s", path, id)
		item, _ := asObject(opts[id])
		b.reportOtherKeys(item, []string{"stat", "timeField"}, itemPath)
		if field := asString(item["timeField"]); field != "" && field != defaultStatsTime {
			b.partial(itemPath+".timeField", "")
		}
		stat := asString(item["stat"])
		ref, mapped := b.refs[id]
		switch {
		case stat == "" || stat == defaultReducer:
		case !isReducer(stat) || !mapped || len(t.Stats) >= maxStats:
			b.partial(itemPath+".stat", "")
		default:
			if t.Stats == nil {
				t.Stats = map[string]string{}
			}
			t.Stats[ref] = stat
		}
	}
	return t, true
}

func (b *panelBuild) configMappings(v any, handlers map[string]string, withReducer bool, path string) []spec.ConfigMapping {
	items, _ := asArray(v)
	var out []spec.ConfigMapping
	for i, item := range items {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		obj, _ := asObject(item)
		field, fieldOK := nameOption(obj, "fieldName")
		handler := asString(obj["handlerKey"])
		to, known := handlers[handler]
		switch {
		case handler == "":
		case !fieldOK:
			b.partial(itemPath, "")
		case !known:
			b.partial(itemPath+".handlerKey", "")
		case len(out) >= maxConfigMappings:
			b.partial(itemPath, fmt.Sprintf("A transformation keeps at most %d mappings.", maxConfigMappings))
		default:
			mapping := spec.ConfigMapping{Field: field, To: to}
			if withReducer {
				mapping.Reducer = b.mappingReducer(obj, itemPath)
			}
			out = append(out, mapping)
		}
		handled := []string{"fieldName", "handlerKey"}
		if withReducer {
			handled = append(handled, "reducerId")
		}
		b.reportOtherKeys(obj, handled, itemPath)
	}
	return out
}

func (b *panelBuild) mappingReducer(obj map[string]any, path string) string {
	reducer := asString(obj["reducerId"])
	switch {
	case reducer == "" || reducer == defaultReducer:
		return ""
	case !isReducer(reducer):
		b.partial(path+".reducerId", "")
		return ""
	}
	return reducer
}

func rowsToFieldsTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "rowsToFields"}
	t.Mappings = b.configMappings(opts["mappings"], rowFieldHandlers, false, path+".options.mappings")
	return t, true
}

func configFromDataTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	ref, mapped := b.refs[cmp.Or(asString(opts["configRefId"]), "config")]
	if !mapped {
		b.partial(path+".options.configRefId", "")
		return spec.Transform{}, false
	}
	t := spec.Transform{Type: "configFromData", ConfigQuery: ref}
	t.Mappings = b.configMappings(opts["mappings"], configFieldHandlers, true, path+".options.mappings")
	if len(t.Mappings) == 0 {
		b.partial(path+".options.mappings", "")
		return spec.Transform{}, false
	}
	if applyTo, _ := asObject(opts["applyTo"]); present(applyTo) {
		if match, ok := b.matchFor(applyTo); ok {
			t.ApplyTo = &match
		} else {
			b.partial(path+".options.applyTo", "")
		}
	}
	return t, true
}

func concatenateTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "concatenate"}
	switch mode := cmp.Or(asString(opts["frameNameMode"]), "drop"); {
	case mode == "drop":
	case slices.Contains([]string{"field", "label"}, mode):
		t.NameMode = mode
	default:
		b.partial(path+".options.frameNameMode", "")
	}
	if label := asString(opts["frameNameLabel"]); t.NameMode == "label" && label != defaultFrameLabel {
		if validLength(label, maxNameMatch) {
			t.NameLabel = label
		} else if label != "" {
			b.partial(path+".options.frameNameLabel", "")
		}
	}
	return t, true
}
