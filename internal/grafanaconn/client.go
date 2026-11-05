package grafanaconn

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"github.com/slashobserve/slashobserve/internal/egress"
)

const (
	searchPageSize   = 5000
	maxSearchPages   = 100
	maxResponseBytes = 8 << 20
	rootFolderUID    = "general"
	rootFolderTitle  = "General"
)

var errBadResponse = errors.New("grafana answered with JSON this server does not understand")

type grafanaClient struct {
	egress *egress.Client
	base   *url.URL
	token  string
}

type apiFolder struct {
	UID   string `json:"uid"`
	Title string `json:"title"`
}

type apiHit struct {
	UID         string   `json:"uid"`
	Title       string   `json:"title"`
	FolderUID   string   `json:"folderUid"`
	FolderTitle string   `json:"folderTitle"`
	Tags        []string `json:"tags"`
}

type apiDatasource struct {
	UID  string `json:"uid"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type apiDashboard struct {
	Dashboard struct {
		Version int `json:"version"`
	} `json:"dashboard"`
}

func (c grafanaClient) get(ctx context.Context, query url.Values, into any, path ...string) ([]byte, error) {
	u := c.base.JoinPath(path...)
	u.RawQuery = query.Encode()
	body, err := c.egress.FetchAuthorized(ctx, egress.AuthorizedRequest{
		Request:       egress.Request{URL: u.String(), Accept: []string{"application/json"}, MaxBytes: maxResponseBytes},
		Authorization: "Bearer " + c.token,
		AllowPrivate:  true,
	})
	if err != nil {
		return nil, err
	}
	if into != nil && json.Unmarshal(body, into) != nil {
		return nil, errBadResponse
	}
	return body, nil
}

func (c grafanaClient) health(ctx context.Context) error {
	var body map[string]any
	_, err := c.get(ctx, nil, &body, "api", "health")
	return err
}

func (c grafanaClient) rootFolders(ctx context.Context) ([]apiFolder, error) {
	var folders []apiFolder
	_, err := c.get(ctx, nil, &folders, "api", "folders")
	return folders, err
}

func (c grafanaClient) searchPage(ctx context.Context, kind string, page int) ([]apiHit, error) {
	query := url.Values{"type": {kind}, "limit": {strconv.Itoa(searchPageSize)}, "page": {strconv.Itoa(page)}}
	var hits []apiHit
	_, err := c.get(ctx, query, &hits, "api", "search")
	return hits, err
}

func (c grafanaClient) searchAll(ctx context.Context, kind string) ([]apiHit, error) {
	var all []apiHit
	seen := map[string]bool{}
	for page := 1; page <= maxSearchPages; page++ {
		hits, err := c.searchPage(ctx, kind, page)
		if err != nil {
			return nil, err
		}
		added := 0
		for _, h := range hits {
			if h.UID != "" && !seen[h.UID] {
				seen[h.UID] = true
				all = append(all, h)
				added++
			}
		}
		if len(hits) < searchPageSize || added == 0 {
			break
		}
	}
	return all, nil
}

func (c grafanaClient) datasources(ctx context.Context) ([]apiDatasource, error) {
	var list []apiDatasource
	_, err := c.get(ctx, nil, &list, "api", "datasources")
	return list, err
}

func (c grafanaClient) dashboard(ctx context.Context, uid string) ([]byte, int, error) {
	var parsed apiDashboard
	body, err := c.get(ctx, nil, &parsed, "api", "dashboards", "uid", uid)
	if err != nil {
		return nil, 0, err
	}
	return body, parsed.Dashboard.Version, nil
}
