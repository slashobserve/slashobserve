package grafana

import (
	"slices"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

const (
	airportsGazetteer = "public/gazetteer/airports.geojson"
	airportsReason    = "Airport lookup is not available."
	gazetteerReason   = "Only the built-in countries and US states gazetteers are supported."
	modeAuto          = "auto"
)

var gazetteerPaths = map[string]string{
	"public/gazetteer/countries.json":  "countries",
	"public/gazetteer/usa-states.json": "usa-states",
}

var (
	spatialActions     = []string{"prepare", "calculate", "modify"}
	spatialCalcs       = []string{"heading", "distance", "area"}
	spatialModifiers   = []string{"lineBuilder", "asLine"}
	locationModes      = []string{modeAuto, "coords", "geohash", "lookup"}
	locationModeFields = map[string][]string{
		modeAuto:  nil,
		"coords":  {"latitude", "longitude"},
		"geohash": {"geohash"},
		"lookup":  {"lookup"},
	}
)

func (b *panelBuild) gazetteerName(v any, path string) (string, bool) {
	file := asString(v)
	if file == "" {
		return "", true
	}
	if name, known := gazetteerPaths[file]; known {
		return name, true
	}
	if file == airportsGazetteer {
		b.partial(path, airportsReason)
	} else {
		b.partial(path, gazetteerReason)
	}
	return "", false
}

func (b *panelBuild) locationNames(raw map[string]any, mode, path string) (spec.SpatialLocation, bool) {
	loc := spec.SpatialLocation{Mode: mode}
	for _, key := range locationModeFields[mode] {
		name, ok := nameOption(raw, key)
		switch {
		case ok:
			setLocationName(&loc, key, name)
		case asString(raw[key]) != "":
			b.partial(path+"."+key, "")
		}
	}
	if mode != "lookup" {
		return loc, true
	}
	gazetteer, ok := b.gazetteerName(raw["gazetteer"], path+".gazetteer")
	loc.Gazetteer = gazetteer
	return loc, ok
}

func setLocationName(loc *spec.SpatialLocation, key, name string) {
	switch key {
	case "latitude":
		loc.Latitude = name
	case "longitude":
		loc.Longitude = name
	case "geohash":
		loc.Geohash = name
	case "lookup":
		loc.Lookup = name
	}
}

func (b *panelBuild) spatialLocation(v any, path string) (*spec.SpatialLocation, bool) {
	raw, _ := asObject(v)
	b.reportOtherKeys(raw, []string{"mode", "latitude", "longitude", "geohash", "lookup", "gazetteer"}, path)
	mode := asString(raw["mode"])
	if mode == "" {
		mode = modeAuto
	}
	if !slices.Contains(locationModes, mode) {
		b.partial(path+".mode", "")
		return nil, true
	}
	if mode == modeAuto {
		return nil, true
	}
	loc, ok := b.locationNames(raw, mode, path)
	return &loc, ok
}

func choice(b *panelBuild, v any, allowed []string, path string) (string, bool) {
	text := asString(v)
	if text == "" {
		return "", true
	}
	if slices.Contains(allowed, text) {
		return text, true
	}
	b.partial(path, "")
	return "", false
}

func (b *panelBuild) spatialCalculation(opts map[string]any, t *spec.Transform, path string) {
	calc, _ := asObject(opts["calculate"])
	b.reportOtherKeys(calc, []string{"calc", "field"}, path)
	t.Calc, _ = choice(b, calc["calc"], spatialCalcs, path+".calc")
	if field, ok := nameOption(calc, "field"); ok {
		t.Field = field
	} else if asString(calc["field"]) != "" {
		b.partial(path+".field", "")
	}
}

func (b *panelBuild) spatialModification(opts map[string]any, t *spec.Transform, path string) bool {
	modify, _ := asObject(opts["modify"])
	b.reportOtherKeys(modify, []string{"op", "target"}, path)
	t.Modify, _ = choice(b, modify["op"], spatialModifiers, path+".op")
	if t.Modify == "asLine" || !present(modify["target"]) {
		return true
	}
	target, ok := b.spatialLocation(modify["target"], path+".target")
	t.Target = target
	return ok
}

func spatialTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "spatial"}
	action, ok := choice(b, opts["action"], spatialActions, path+".options.action")
	if !ok {
		return spec.Transform{}, false
	}
	if action != "prepare" {
		t.Action = action
	}
	source, ok := b.spatialLocation(opts["source"], path+".options.source")
	if !ok {
		return spec.Transform{}, false
	}
	t.Source = source
	switch action {
	case "calculate":
		b.spatialCalculation(opts, &t, path+".options.calculate")
	case "modify":
		if !b.spatialModification(opts, &t, path+".options.modify") {
			return spec.Transform{}, false
		}
	}
	return t, true
}

func fieldLookupTransform(b *panelBuild, opts map[string]any, path string) (spec.Transform, bool) {
	t := spec.Transform{Type: "fieldLookup"}
	if field, ok := nameOption(opts, "lookupField"); ok {
		t.LookupField = field
	} else if asString(opts["lookupField"]) != "" {
		b.partial(path+".options.lookupField", "")
	}
	gazetteer, ok := b.gazetteerName(opts["gazetteer"], path+".options.gazetteer")
	if !ok {
		return spec.Transform{}, false
	}
	t.Gazetteer = gazetteer
	return t, true
}
