package elasticsearch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/slashobserve/slashobserve/internal/datasource"
)

const (
	minElasticsearchMajor = 8
	maxElasticsearchMajor = 9
	minOpenSearchMajor    = 2
	maxOpenSearchMajor    = 3
)

func (d *Datasource) CheckHealth(ctx context.Context) error {
	ctx, cancel := bound(ctx, d.opts.Limits.Timeout)
	defer cancel()
	if err := d.checkServer(ctx); err != nil {
		return err
	}
	tables, err := d.resolveIndex(ctx)
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return datasource.Errorf(datasource.ErrNoIndex, "the index pattern matches no index")
	}
	_, err = d.Roles(ctx)
	return err
}

func (d *Datasource) checkServer(ctx context.Context) error {
	raw, err := d.call(ctx, http.MethodGet, "/", nil, nil)
	if err != nil {
		return err
	}
	var root struct {
		Version struct {
			Number       string `json:"number"`
			Distribution string `json:"distribution"`
		} `json:"version"`
	}
	if json.Unmarshal(raw, &root) != nil || root.Version.Number == "" {
		return unreadable()
	}
	flavor := Elasticsearch
	if root.Version.Distribution == string(OpenSearch) {
		flavor = OpenSearch
	}
	if flavor != d.opts.Flavor {
		if flavor == OpenSearch {
			return datasource.Errorf(datasource.ErrIsOpenSearch, "the server is OpenSearch")
		}
		return datasource.Errorf(datasource.ErrIsElasticsearch, "the server is Elasticsearch")
	}
	if !supportedVersion(flavor, root.Version.Number) {
		return datasource.Errorf(datasource.ErrUnsupportedVersion, "%s %s is not supported", flavor, truncateText(root.Version.Number))
	}
	return nil
}

func supportedVersion(flavor Flavor, number string) bool {
	major, err := strconv.Atoi(strings.SplitN(number, ".", 2)[0])
	if err != nil {
		return false
	}
	if flavor == OpenSearch {
		return major >= minOpenSearchMajor && major <= maxOpenSearchMajor
	}
	return major >= minElasticsearchMajor && major <= maxElasticsearchMajor
}

func (d *Datasource) resolveIndex(ctx context.Context) ([]datasource.Table, error) {
	raw, err := d.call(ctx, http.MethodGet, "/_resolve/index/"+url.PathEscape(d.opts.Index), url.Values{"expand_wildcards": {"open"}}, nil)
	if err != nil {
		return nil, err
	}
	type named struct {
		Name string `json:"name"`
	}
	var resolved struct {
		Indices     []named `json:"indices"`
		Aliases     []named `json:"aliases"`
		DataStreams []named `json:"data_streams"`
	}
	if err := json.Unmarshal(raw, &resolved); err != nil {
		return nil, unreadable()
	}
	var tables []datasource.Table
	for _, group := range []struct {
		kind  string
		items []named
	}{{"index", resolved.Indices}, {"alias", resolved.Aliases}, {"data_stream", resolved.DataStreams}} {
		for _, item := range group.items {
			tables = append(tables, datasource.Table{Name: item.Name, Kind: group.kind})
		}
	}
	slices.SortFunc(tables, func(a, b datasource.Table) int { return strings.Compare(a.Name, b.Name) })
	return tables, nil
}
