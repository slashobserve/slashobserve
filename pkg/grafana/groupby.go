package grafana

import (
	"fmt"
	"slices"

	"github.com/slashobserve/slashobserve/pkg/spec"
)

func (c *converter) groupByVariable(raw map[string]any, path, name string) (spec.Variable, bool) {
	subject := "variable " + name
	ref := c.reg.lookup(raw["datasource"])
	switch {
	case !ref.supported():
		c.dropVariable(subject, path+".datasource", fmt.Sprintf("It groups %s, which is not supported yet.", ref.label()))
		return spec.Variable{}, false
	case ref.src.kind != kindPrometheus:
		c.dropVariable(subject, path+".datasource", "Group-by variables on this datasource are not supported.")
		return spec.Variable{}, false
	}
	v := spec.Variable{Type: spec.GroupByVariable, Datasource: ref.src.ref()}
	if keys := c.groupByDefault(raw, path, subject); len(keys) > 0 {
		v.Default = &spec.Selection{Values: keys, List: true}
	}
	if multi, ok := raw["multi"].(bool); ok && !multi {
		c.note(subject, "A group-by variable here always selects several labels.", path+".multi")
	}
	if options, _ := asArray(raw["options"]); len(options) > 0 {
		c.note(subject, "Saved label options are not kept: the picker offers the datasource's labels.", path+".options")
	}
	return v, true
}

func (c *converter) groupByDefault(raw map[string]any, path, subject string) []string {
	current := groupByKeys(raw["current"])
	saved := groupByKeys(raw["defaultValue"])
	picked, at := current, path+".current"
	if len(picked) == 0 {
		picked, at = saved, path+".defaultValue"
	} else if len(saved) > 0 && !slices.Equal(saved, current) {
		c.note(subject, "The saved default selection is not kept: the current selection is the default.", path+".defaultValue")
	}
	valid := make([]string, 0, len(picked))
	for _, key := range picked {
		if !spec.IsGroupByKey(key) {
			c.note(subject, "A saved label is not a valid label name, so it is not kept.", at)
			continue
		}
		valid = append(valid, key)
	}
	if len(valid) > spec.MaxGroupByKeys {
		c.note(subject, fmt.Sprintf("SlashObserve keeps at most %d saved labels per variable.", spec.MaxGroupByKeys), at)
		valid = valid[:spec.MaxGroupByKeys]
	}
	return valid
}

func groupByKeys(selection any) []string {
	object, _ := asObject(selection)
	var values []string
	switch value := object["value"].(type) {
	case string:
		values = []string{value}
	case []any:
		values = stringList(value)
	}
	var keys []string
	for _, value := range values {
		if value != "" && !slices.Contains(keys, value) {
			keys = append(keys, value)
		}
	}
	return keys
}
