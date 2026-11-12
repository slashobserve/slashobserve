package dashboards

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/db/dbsqlc"
	"github.com/slashobserve/slashobserve/internal/folders"
	"github.com/slashobserve/slashobserve/internal/grantmode"
	"github.com/slashobserve/slashobserve/internal/origin"
	"github.com/slashobserve/slashobserve/pkg/spec"
)

const maxMergeAttempts = 3

type SaveInput struct {
	DashboardID     pgtype.UUID
	Parent          int
	Spec            json.RawMessage
	Summary         string
	RequestID       pgtype.UUID
	Partial         bool
	Merge           bool
	ProposeIfNeeded bool
	Choices         []spec.Choice
	MergedProposal  pgtype.UUID
	InTx            func(q *dbsqlc.Queries, version int) error
}

type MergedFrom struct {
	BaseVersion int `json:"base_version"`
	OntoVersion int `json:"onto_version"`
}

type SaveResult struct {
	Dashboard  Dashboard
	Version    Version
	Replayed   bool
	MergedFrom *MergedFrom
	Proposal   *ProposalRef
}

type ProposeInput struct {
	DashboardID pgtype.UUID
	BaseVersion int
	Spec        *spec.Dashboard
	Summary     string
	RequestID   pgtype.UUID
	Partial     bool
	InTx        func(q *dbsqlc.Queries) error
}

type ProposalRef struct {
	ID       string
	Revision int
	Opened   bool
}

type Proposer func(ctx context.Context, by auth.Principal, in ProposeInput) (ProposalRef, error)

func (s *Service) SetProposer(p Proposer) {
	s.proposer = p
}

type ProposalRequiredError struct {
	Removed []spec.UnitChange
}

func (e *ProposalRequiredError) Error() string {
	units := make([]string, len(e.Removed))
	for i, u := range e.Removed {
		units[i] = fmt.Sprintf("%s %q", u.Kind, u.ID)
	}
	return "This connection is in propose mode, and a proposal cannot remove panels or variables; ask your person to remove " + strings.Join(units, ", ")
}

func (s *Service) refuseRemovals(ctx context.Context, id pgtype.UUID, base int, next *spec.Dashboard) error {
	if grantmode.From(ctx) != grantmode.Propose {
		return nil
	}
	parent, err := s.VersionSpec(ctx, id, base)
	if err != nil {
		return err
	}
	var removed []spec.UnitChange
	for _, c := range spec.Changes(parent, next) {
		if c.Change == spec.ChangeRemoved && (c.Kind == spec.UnitPanel || c.Kind == spec.UnitVariable) {
			removed = append(removed, c)
		}
	}
	if len(removed) == 0 {
		return nil
	}
	return &ProposalRequiredError{Removed: removed}
}

type savedDocument struct {
	summary   string
	parsed    *spec.Dashboard
	canonical []byte
}

func mustPropose(ctx context.Context, a folders.Abilities) bool {
	return !a.Edit || grantmode.From(ctx) == grantmode.Propose
}

func (s *Service) Save(ctx context.Context, by auth.Principal, in SaveInput) (SaveResult, error) {
	access, err := s.need(ctx, by, in.DashboardID, folders.NeedView)
	if err != nil {
		return SaveResult{}, err
	}
	if mustPropose(ctx, access.Abilities) {
		return s.propose(ctx, by, in, access)
	}
	res, err := s.persist(ctx, by, in)
	if err != nil {
		return SaveResult{}, err
	}
	res.Dashboard = withAccess(res.Dashboard, access)
	return res, nil
}

func (s *Service) propose(ctx context.Context, by auth.Principal, in SaveInput, access folders.DashboardAccess) (SaveResult, error) {
	if !in.ProposeIfNeeded || s.proposer == nil || !access.Abilities.Comment {
		return SaveResult{}, folders.ErrForbidden
	}
	summary, parsed, _, err := validateSave(in.Spec, in.Summary, "")
	if err != nil {
		return SaveResult{}, err
	}
	if err := s.refuseRemovals(ctx, in.DashboardID, in.Parent, parsed); err != nil {
		return SaveResult{}, err
	}
	ref, err := s.proposer(ctx, by, ProposeInput{
		DashboardID: in.DashboardID,
		BaseVersion: in.Parent,
		Spec:        parsed,
		Summary:     summary,
		RequestID:   in.RequestID,
		Partial:     in.Partial,
	})
	if err != nil {
		return SaveResult{}, err
	}
	dash, _, err := s.store.latest(ctx, in.DashboardID)
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{Dashboard: withAccess(dash, access), Proposal: &ref}, nil
}

func (s *Service) CheckProposal(ctx context.Context, by auth.Principal, id pgtype.UUID, base int, next *spec.Dashboard, partial bool) error {
	prev, err := s.VersionSpec(ctx, id, base)
	if err != nil {
		return err
	}
	prevUnverified, err := s.store.unverified(ctx, id, base)
	if err != nil {
		return err
	}
	_, err = s.verify(ctx, by, prev, prevUnverified, next, partial)
	return err
}

func (s *Service) persist(ctx context.Context, by auth.Principal, in SaveInput) (SaveResult, error) {
	if in.RequestID.Valid {
		dash, ver, found, err := s.store.savedByRequest(ctx, in.RequestID, in.DashboardID)
		if err != nil {
			return SaveResult{}, err
		}
		if found {
			return SaveResult{Dashboard: dash, Version: ver, Replayed: true}, nil
		}
	}
	summary, parsed, canonical, err := validateSave(in.Spec, in.Summary, "")
	if err != nil {
		return SaveResult{}, err
	}
	latest, err := s.store.latestVersion(ctx, in.DashboardID)
	if err != nil {
		return SaveResult{}, err
	}
	if in.Parent >= 1 && in.Parent <= latest {
		prev, err := s.VersionSpec(ctx, in.DashboardID, in.Parent)
		if err != nil {
			return SaveResult{}, err
		}
		if freezeResized(prev, parsed) {
			if canonical, err = json.Marshal(parsed); err != nil {
				return SaveResult{}, fmt.Errorf("marshal spec: %w", err)
			}
		}
	}
	doc := savedDocument{summary: summary, parsed: parsed, canonical: canonical}
	if latest != in.Parent {
		return s.resolveStale(ctx, by, in, doc)
	}
	dash, ver, err := s.commit(ctx, by, in, doc, in.Parent)
	if errors.Is(err, errStaleParent) || errors.Is(err, errRequestSaved) {
		return s.resolveStale(ctx, by, in, doc)
	}
	if err != nil {
		return SaveResult{}, err
	}
	return SaveResult{Dashboard: dash, Version: ver}, nil
}

func (s *Service) commit(ctx context.Context, by auth.Principal, in SaveInput, doc savedDocument, parent int) (Dashboard, Version, error) {
	check := s.checkSave
	if in.Partial {
		check = s.checkChangedSave
	}
	carried, err := check(ctx, by, in.DashboardID, parent, doc.parsed)
	if err != nil {
		return Dashboard{}, Version{}, err
	}
	var inTx func(q *dbsqlc.Queries, dash Dashboard, ver Version) error
	if in.InTx != nil {
		inTx = func(q *dbsqlc.Queries, _ Dashboard, ver Version) error {
			return in.InTx(q, ver.Number)
		}
	}
	return s.store.addVersionWith(ctx, newVersion{
		dashboardID:    in.DashboardID,
		parent:         parent,
		spec:           doc.parsed,
		canonical:      doc.canonical,
		summary:        doc.summary,
		author:         authorOf(by),
		requestID:      in.RequestID,
		origin:         origin.From(ctx),
		unverified:     carried,
		mergedProposal: in.MergedProposal,
	}, inTx)
}

func (s *Service) resolveStale(ctx context.Context, by auth.Principal, in SaveInput, doc savedDocument) (SaveResult, error) {
	dash, ver, replayed, err := s.replayOrConflict(ctx, by, in.DashboardID, in.Parent, doc.canonical, in.RequestID)
	if err == nil {
		return SaveResult{Dashboard: dash, Version: ver, Replayed: replayed}, nil
	}
	var conflict *ConflictError
	if !in.Merge || !errors.As(err, &conflict) || in.Parent < 1 || in.Parent >= conflict.Latest {
		return SaveResult{}, err
	}
	return s.mergeOnto(ctx, by, in, doc)
}

func (s *Service) mergeOnto(ctx context.Context, by auth.Principal, in SaveInput, doc savedDocument) (SaveResult, error) {
	base, err := s.VersionSpec(ctx, in.DashboardID, in.Parent)
	if err != nil {
		return SaveResult{}, err
	}
	for range maxMergeAttempts {
		dash, latest, err := s.store.latest(ctx, in.DashboardID)
		if err != nil {
			return SaveResult{}, err
		}
		merge, err := spec.Merge3(base, latest.Spec, doc.parsed, in.Choices...)
		if errors.Is(err, spec.ErrUnknownChoice) {
			return SaveResult{}, &InvalidInputError{Message: err.Error()}
		}
		if err != nil {
			return SaveResult{}, err
		}
		if len(merge.Conflicts) > 0 {
			return SaveResult{}, &ConflictError{Latest: latest.Number, Conflicts: merge.Conflicts}
		}
		from := &MergedFrom{BaseVersion: in.Parent, OntoVersion: latest.Number}
		merged, err := marshalSpec(merge.Dashboard)
		if err != nil {
			return SaveResult{}, err
		}
		current, err := marshalSpec(latest.Spec)
		if err != nil {
			return SaveResult{}, err
		}
		if bytes.Equal(merged, current) {
			return SaveResult{Dashboard: dash, Version: latest, Replayed: true, MergedFrom: from}, nil
		}
		next := savedDocument{summary: doc.summary, parsed: merge.Dashboard, canonical: merged}
		dash, ver, err := s.commit(ctx, by, in, next, latest.Number)
		switch {
		case errors.Is(err, errStaleParent):
			continue
		case errors.Is(err, errRequestSaved):
			return s.requestReplay(ctx, in)
		case err != nil:
			return SaveResult{}, err
		}
		return SaveResult{Dashboard: dash, Version: ver, MergedFrom: from}, nil
	}
	return SaveResult{}, s.conflict(ctx, in.DashboardID)
}

func (s *Service) requestReplay(ctx context.Context, in SaveInput) (SaveResult, error) {
	dash, ver, found, err := s.store.savedByRequest(ctx, in.RequestID, in.DashboardID)
	if err != nil {
		return SaveResult{}, err
	}
	if !found {
		return SaveResult{}, s.conflict(ctx, in.DashboardID)
	}
	return SaveResult{Dashboard: dash, Version: ver, Replayed: true}, nil
}

func marshalSpec(d *spec.Dashboard) ([]byte, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	return raw, nil
}
