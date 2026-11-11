package mcp

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/slashobserve/slashobserve/internal/agent/prompts"
	"github.com/slashobserve/slashobserve/internal/agent/prompts/skill"
	"github.com/slashobserve/slashobserve/internal/auth"
	"github.com/slashobserve/slashobserve/internal/revisions"
)

const (
	observePromptDescription = "Build or change a dashboard, investigate a question, search logs or traces, or answer comments, the way SlashObserve's own agent works."
	observeAskDescription    = "What you want, for example: checkout health by pod, last 6h."
	observeKindDescription   = "Optional. build or investigate: return the system text the server's own agent works from for that kind of run, the text a claimed run expects; leave it out for the general guide."
)

func observePrompt() mcplib.Prompt {
	return mcplib.NewPrompt(skill.Name,
		mcplib.WithPromptTitle("Observe"),
		mcplib.WithPromptDescription(observePromptDescription),
		mcplib.WithArgument("ask", mcplib.ArgumentDescription(observeAskDescription)),
		mcplib.WithArgument("kind", mcplib.ArgumentDescription(observeKindDescription)))
}

func (h *handler) observePromptHandler() mcpserver.PromptHandlerFunc {
	return func(ctx context.Context, req mcplib.GetPromptRequest) (*mcplib.GetPromptResult, error) {
		principal, ok := auth.FromContext(ctx)
		if !ok {
			h.logger.ErrorContext(ctx, "mcp prompt without principal", slog.String("prompt", skill.Name))
			return nil, errors.New(internalErrorMsg)
		}
		kind := strings.TrimSpace(req.Params.Arguments["kind"])
		var text string
		attrs := []slog.Attr{
			slog.String("prompt", skill.Name),
			slog.String("principal_kind", string(principal.Kind)),
			slog.String("principal_id", principal.ID),
		}
		switch kind {
		case "":
			text = skill.Prompt(req.Params.Arguments["ask"])
		case revisions.KindBuild:
			text = prompts.Build(h.types())
			attrs = append(attrs, slog.String("kind", kind))
		case revisions.KindInvestigate:
			text = prompts.Investigate()
			attrs = append(attrs, slog.String("kind", kind))
		default:
			return nil, errors.New("kind must be build or investigate")
		}
		h.logger.LogAttrs(ctx, slog.LevelInfo, "mcp prompt", attrs...)
		message := mcplib.NewPromptMessage(mcplib.RoleUser, mcplib.NewTextContent(text))
		return mcplib.NewGetPromptResult(observePromptDescription, []mcplib.PromptMessage{message}), nil
	}
}
