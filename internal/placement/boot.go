package placement

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type MisplacedError struct {
	Datasource string
	Count      int
	Items      []Holder
}

func (e *MisplacedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "the datasources file makes %q Admins only, but %d dashboards, proposals, observations or saved searches outside folders only Admins can read use it:", e.Datasource, e.Count)
	for _, h := range e.Items {
		where := fmt.Sprintf("in folder %q", h.FolderTitle)
		if h.Personal {
			where = "in its owner's My Dashboards"
		}
		fmt.Fprintf(&b, " %s %s %q %s;", h.Kind.Noun(), h.ID, h.Title, where)
	}
	if hidden := e.Count - len(e.Items); hidden > 0 {
		fmt.Fprintf(&b, " and %d more;", hidden)
	}
	b.WriteString(" move them into a folder only Admins can read while the datasource is still everyone, or keep audience: everyone in the file")
	return b.String()
}

func CheckFileAudiences(ctx context.Context, q *dbsqlc.Queries, adminsOnly []string) error {
	for _, uid := range adminsOnly {
		holders, err := scan(ctx, q, scope{uids: []string{uid}, sharedOnly: true, limit: math.MaxInt32})
		if err != nil {
			return err
		}
		if len(holders) > 0 {
			return &MisplacedError{Datasource: uid, Count: len(holders), Items: holders[:min(len(holders), MaxListed)]}
		}
	}
	return nil
}
