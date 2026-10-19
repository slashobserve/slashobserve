package grafana

import (
	"reflect"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func dropDefaults(d *spec.Dashboard) {
	table := spec.Defaults()
	for i := range d.Variables {
		if d.Variables[i].Refresh == table.Variable.Refresh {
			d.Variables[i].Refresh = ""
		}
	}
	for i := range d.Panels {
		dropPanelDefaults(&d.Panels[i], table)
		for j := range d.Panels[i].Panels {
			dropPanelDefaults(&d.Panels[i].Panels[j], table)
		}
	}
}

func dropPanelDefaults(p *spec.Panel, table spec.DefaultsTable) {
	for i := range p.Queries {
		if p.Queries[i].Format == table.Query.Format {
			p.Queries[i].Format = ""
		}
	}
	f, want := &p.FieldProps, table.FieldProps
	if f.ThresholdMode == want.ThresholdMode {
		f.ThresholdMode = ""
	}
	if f.Style == want.Style {
		f.Style = ""
	}
	if f.Fill != nil && *f.Fill == want.Fill {
		f.Fill = nil
	}
	if f.LineStyle == want.LineStyle {
		f.LineStyle = ""
	}
	if f.Stack == want.Stack {
		f.Stack = ""
	}
	if f.Axis == want.Axis {
		f.Axis = ""
	}
	if f.Hidden != nil && *f.Hidden == want.Hidden {
		f.Hidden = nil
	}
	if p.Display != nil {
		if def, ok := table.Display[p.Type]; ok {
			dropDisplayDefaults(p.Display, def)
		}
		if reflect.ValueOf(*p.Display).IsZero() {
			p.Display = nil
		}
	}
}

func dropDisplayDefaults(d *spec.Display, def spec.Display) {
	got, want := reflect.ValueOf(d).Elem(), reflect.ValueOf(def)
	for i := range got.NumField() {
		field, defaultValue := got.Field(i), want.Field(i)
		if defaultValue.IsZero() {
			continue
		}
		switch field.Kind() {
		case reflect.Pointer:
			if !field.IsNil() && reflect.DeepEqual(field.Elem().Interface(), defaultValue.Elem().Interface()) {
				field.SetZero()
			}
		case reflect.Slice:
			if field.Len() == defaultValue.Len() && (field.Len() == 0 || reflect.DeepEqual(field.Interface(), defaultValue.Interface())) {
				field.SetZero()
			}
		default:
			if reflect.DeepEqual(field.Interface(), defaultValue.Interface()) {
				field.SetZero()
			}
		}
	}
}
