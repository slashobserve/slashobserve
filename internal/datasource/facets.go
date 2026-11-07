package datasource

import (
	"cmp"
	"context"
	"slices"
	"strings"
)

const (
	FacetKindLabel    = "label"
	FacetKindField    = "field"
	FacetKindMetadata = "metadata"
	FacetKindColumn   = "column"
)

type FacetRequest struct {
	Query  Query
	Range  TimeRange
	Fields []string
	Limit  int
	Sample int
}

type FacetValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

type FacetField struct {
	Name        string       `json:"name"`
	Kind        string       `json:"kind"`
	Exact       bool         `json:"exact"`
	Unit        string       `json:"unit,omitempty"`
	Cardinality int          `json:"cardinality,omitempty"`
	Values      []FacetValue `json:"values"`
	Truncated   bool         `json:"truncated"`
}

type Facets struct {
	Sampled      bool         `json:"sampled"`
	LinesSampled int          `json:"lines_sampled"`
	Fields       []FacetField `json:"fields"`
	Truncated    bool         `json:"truncated"`
}

type LogFacetSource interface {
	LogFacets(ctx context.Context, req FacetRequest) (Facets, error)
}

func SortFacets(f *Facets) {
	slices.SortStableFunc(f.Fields, func(a, b FacetField) int {
		if a.Exact != b.Exact {
			if a.Exact {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(a.Cardinality, b.Cardinality), strings.Compare(a.Name, b.Name))
	})
	for i := range f.Fields {
		slices.SortStableFunc(f.Fields[i].Values, func(a, b FacetValue) int {
			return cmp.Or(cmp.Compare(b.Count, a.Count), strings.Compare(a.Value, b.Value))
		})
	}
}

func TrimFacets(f *Facets, maxFields, maxValues int) {
	if len(f.Fields) > maxFields {
		f.Fields, f.Truncated = f.Fields[:maxFields], true
	}
	for i := range f.Fields {
		if len(f.Fields[i].Values) > maxValues {
			f.Fields[i].Values, f.Fields[i].Truncated = f.Fields[i].Values[:maxValues], true
		}
	}
}
