package tempo

import (
	"context"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	discoverTimeout = 10 * time.Second
	intrinsicScope  = "intrinsic"
)

var tagName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.:-]*$`)

type tagsResponse struct {
	Scopes []struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	} `json:"scopes"`
}

type tagValuesResponse struct {
	TagValues []struct {
		Value string `json:"value"`
	} `json:"tagValues"`
}

func (d *Datasource) Discover(ctx context.Context, req datasource.DiscoverRequest) (*datasource.Catalog, error) {
	switch req.Kind {
	case datasource.DiscoverLabels, datasource.DiscoverLabelValues:
	default:
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "discover kind %q is not supported by tempo", truncateText(string(req.Kind)))
	}
	ctx, cancel := bound(ctx, discoverTimeout)
	defer cancel()
	limit := req.EffectiveLimit()
	if req.Kind == datasource.DiscoverLabels {
		var resp tagsResponse
		if err := d.getJSON(ctx, "/api/v2/search/tags", nil, &resp); err != nil {
			return nil, err
		}
		var names []string
		for _, scope := range resp.Scopes {
			for _, tag := range scope.Tags {
				if scope.Name == intrinsicScope {
					names = append(names, tag)
				} else {
					names = append(names, scope.Name+"."+tag)
				}
			}
		}
		return &datasource.Catalog{Labels: filterSortLimit(names, req.Search, limit)}, nil
	}
	if !tagName.MatchString(req.Label) {
		return nil, datasource.Errorf(datasource.ErrInvalidQuery, "tag must be a scoped TraceQL attribute such as resource.service.name")
	}
	params := url.Values{}
	if req.Metric != "" {
		params.Set("q", req.Metric)
	}
	var resp tagValuesResponse
	if err := d.getJSON(ctx, "/api/v2/search/tag/"+url.PathEscape(req.Label)+"/values", params, &resp); err != nil {
		return nil, err
	}
	values := make([]string, 0, len(resp.TagValues))
	for _, v := range resp.TagValues {
		values = append(values, v.Value)
	}
	return &datasource.Catalog{Values: filterSortLimit(values, req.Search, limit)}, nil
}

func (d *Datasource) VariableValues(context.Context, datasource.Query, datasource.TimeRange) ([]string, error) {
	return nil, datasource.Errorf(datasource.ErrInvalidQuery, "query variables are not supported by tempo")
}

func filterSortLimit(values []string, search string, limit int) []string {
	search = strings.ToLower(search)
	out := make([]string, 0, len(values))
	for _, v := range values {
		if search == "" || strings.Contains(strings.ToLower(v), search) {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}
