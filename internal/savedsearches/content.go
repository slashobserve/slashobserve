package savedsearches

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
)

type contentChange struct {
	query    pgtype.Text
	filters  []byte
	from, to string
	parts    []string
}

func (c contentChange) params(id pgtype.UUID) dbsqlc.ChangeSavedSearchContentParams {
	return dbsqlc.ChangeSavedSearchContentParams{Query: c.query, Filters: c.filters, TimeFrom: c.from, TimeTo: c.to, ID: id}
}

func mergedInput(current dbsqlc.SavedSearch, in UpdateInput) (CreateInput, error) {
	merged := CreateInput{
		Kind:          Kind(current.Kind),
		Title:         current.Title,
		DatasourceUID: current.DatasourceUid,
		Filters:       current.Filters,
		From:          current.TimeFrom,
		To:            current.TimeTo,
	}
	if current.Query.Valid {
		merged.Query = &current.Query.String
	}
	if in.Query != nil {
		merged.Query = in.Query
		if *in.Query == "" {
			merged.Query = nil
		}
	}
	if len(in.Filters) > 0 {
		merged.Filters = in.Filters
	}
	for _, t := range []struct {
		name string
		set  *string
		into *string
	}{{"from", in.From, &merged.From}, {"to", in.To, &merged.To}} {
		if t.set == nil {
			continue
		}
		if *t.set == "" {
			return merged, invalid("%s must not be empty", t.name)
		}
		*t.into = *t.set
	}
	return merged, nil
}

func (s *Service) mergeContent(ctx context.Context, current dbsqlc.SavedSearch, in UpdateInput, now time.Time) (*contentChange, error) {
	merged, err := mergedInput(current, in)
	if err != nil {
		return nil, err
	}
	checked, err := validate(merged, now)
	if err != nil {
		return nil, err
	}
	if err := s.checkDatasource(ctx, checked); err != nil {
		return nil, err
	}
	return changedContent(current, checked)
}

func changedContent(current dbsqlc.SavedSearch, checked validated) (*contentChange, error) {
	filters, err := json.Marshal(checked.filters)
	if err != nil {
		return nil, err
	}
	before, err := savedSearchOf(current)
	if err != nil {
		return nil, err
	}
	out := &contentChange{filters: filters, from: checked.From, to: checked.To}
	if checked.Query != nil {
		out.query = pgtype.Text{String: *checked.Query, Valid: true}
	}
	if !sameText(before.Query, checked.Query) {
		out.parts = append(out.parts, "query")
	}
	if previous, _ := json.Marshal(before.Filters); string(previous) != string(filters) {
		out.parts = append(out.parts, "filters")
	}
	if before.From != checked.From || before.To != checked.To {
		out.parts = append(out.parts, "range")
	}
	if len(out.parts) == 0 {
		return nil, nil
	}
	return out, nil
}

func sameText(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
