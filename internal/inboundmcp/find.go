package inboundmcp

import (
	"context"
	"errors"
	"slices"

	"github.com/slashobserve/slashobserve/internal/agent/tools"
	"github.com/slashobserve/slashobserve/internal/datasource"
	"github.com/slashobserve/slashobserve/internal/queryproxy"
)

const maxFoundDescription = 600

func (s *Service) Find(ctx context.Context, query, uid string, limit int) (tools.ContextFound, error) {
	access, ok := datasource.AccessFrom(ctx)
	if !ok {
		return tools.ContextFound{}, datasource.ErrNoAccess
	}
	candidates, err := s.searchable(ctx, access, uid)
	if err != nil {
		return tools.ContextFound{}, err
	}
	ranked := rank(query, candidates)
	found := tools.ContextFound{Total: len(ranked)}
	for _, hit := range ranked[:min(limit, len(ranked))] {
		datasource.NoteRead(ctx, hit.source.Audience)
		found.Matches = append(found.Matches, matchOf(hit))
	}
	return found, nil
}

func (s *Service) searchable(ctx context.Context, access datasource.Access, uid string) ([]searchable, error) {
	sources, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	if uid != "" {
		i := slices.IndexFunc(sources, func(source Source) bool { return source.UID == uid })
		if i < 0 {
			return nil, notFound(uid)
		}
		if err := readable(access, sources[i].Audience, uid); err != nil {
			return nil, err
		}
		sources = sources[i : i+1]
	}
	var out []searchable
	for _, source := range sources {
		if source.Tools.Approved == 0 || !access.Reads(source.Audience) {
			continue
		}
		stored, err := s.Tools(ctx, source.UID)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, tool := range stored {
			if tool.State == ToolApproved {
				out = append(out, searchableOf(source, tool))
			}
		}
	}
	return out, nil
}

func matchOf(hit scored) tools.ContextMatch {
	return tools.ContextMatch{
		Source:      hit.source.UID,
		SourceName:  hit.source.Name,
		Name:        hit.tool.Name,
		Description: queryproxy.ClipBytes(hit.tool.Pinned.Description, maxFoundDescription),
		Arguments:   argumentsOf(hit.tool.Pinned.InputSchema),
		InputSchema: hit.tool.Pinned.InputSchema,
	}
}
