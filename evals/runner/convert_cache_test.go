package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"

	"github.com/slashobserve/slashobserve/pkg/grafana"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

type conversionKey struct {
	source  [sha256.Size]byte
	options string
}

type conversion struct {
	once      sync.Once
	result    *grafana.Result
	err       error
	cloneable bool
}

var conversions sync.Map

func cloneDashboard(d *spec.Dashboard) (*spec.Dashboard, error) {
	data, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	var out spec.Dashboard
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func init() {
	convertDashboard = func(source []byte, opts grafana.Options) (*grafana.Result, error) {
		entry, _ := conversions.LoadOrStore(conversionKey{sha256.Sum256(source), fmt.Sprintf("%#v", opts)}, &conversion{})
		c := entry.(*conversion)
		c.once.Do(func() {
			c.result, c.err = grafana.Convert(source, opts)
			if c.err != nil || c.result.Spec == nil {
				return
			}
			clone, err := cloneDashboard(c.result.Spec)
			c.cloneable = err == nil && reflect.DeepEqual(clone, c.result.Spec)
		})
		if c.err != nil || !c.cloneable {
			return grafana.Convert(source, opts)
		}
		clone, err := cloneDashboard(c.result.Spec)
		if err != nil {
			return nil, err
		}
		res := *c.result
		res.Spec = clone
		return &res, nil
	}
}
