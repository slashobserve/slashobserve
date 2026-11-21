package placement

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type MisplacedError struct {
	Datasource string
	Census
}

func (e *MisplacedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "the datasources file makes %q Admins only, but %d dashboards, proposals, observations or saved searches outside folders only Admins can read use it:", e.Datasource, e.Shared+e.Personal)
	for _, h := range e.Listed {
		fmt.Fprintf(&b, " %s %s %q in folder %q;", h.Kind.Noun(), h.ID, h.Title, h.FolderTitle)
	}
	if more := e.Unlisted(); more > 0 {
		fmt.Fprintf(&b, " and %d more;", more)
	}
	if e.Personal > 0 {
		fmt.Fprintf(&b, " %d in people's My Dashboards, which only their owners can move;", e.Personal)
	}
	b.WriteString(" move them into a folder only Admins can read while the datasource is still everyone, or keep audience: everyone in the file")
	return b.String()
}

func CheckFileAudiences(ctx context.Context, q *dbsqlc.Queries, adminsOnly []string) error {
	for _, uid := range adminsOnly {
		c, err := SharedCensus(ctx, q, uid)
		if err != nil {
			return err
		}
		if c.Held() {
			return &MisplacedError{Datasource: uid, Census: c}
		}
	}
	return nil
}

type Exposure struct {
	Datasource string
	Shared     int
	Personal   int
}

func ExposedAppDatasources(ctx context.Context, q *dbsqlc.Queries, fileUIDs []string) ([]Exposure, error) {
	stored, err := q.AdminsOnlyDatasourceUIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Admins-only datasources: %w", err)
	}
	var exposed []Exposure
	for _, uid := range stored {
		if slices.Contains(fileUIDs, uid) {
			continue
		}
		c, err := SharedCensus(ctx, q, uid)
		if err != nil {
			return nil, err
		}
		if c.Held() {
			exposed = append(exposed, Exposure{Datasource: uid, Shared: c.Shared, Personal: c.Personal})
		}
	}
	return exposed, nil
}
