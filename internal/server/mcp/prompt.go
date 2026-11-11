package mcp

import (
	"context"
	"errors"
	"log/slog"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/slashobserve/slashobserve/internal/agent/prompts/skill"
	"github.com/slashobserve/slashobserve/internal/auth"
)

const (
	observePromptDescription = "Build or change a dashboard, investigate a question, search logs or traces, or answer comments, the way SlashObserve's own agent works."
	observeAskDescription    = "What you want, for example: checkout health by pod, last 6h."
)

func observePrompt() mcplib.Prompt {
	return mcplib.NewPrompt(skill.Name,
		mcplib.WithPromptTitle("Observe"),
		mcplib.WithPromptDescription(observePromptDescription),
		mcplib.WithArgument("ask", mcplib.ArgumentDescription(observeAskDescription)))
}

func (h *handler) observePromptHandler() mcpserver.PromptHandlerFunc {
	return func(ctx context.Context, req mcplib.GetPromptRequest) (*mcplib.GetPromptResult, error) {
		principal, ok := auth.FromContext(ctx)
		if !ok {
			h.logger.ErrorContext(ctx, "mcp prompt without principal", slog.String("prompt", skill.Name))
			return nil, errors.New(internalErrorMsg)
		}
		h.logger.InfoContext(ctx, "mcp prompt",
			slog.String("prompt", skill.Name),
			slog.String("principal_kind", string(principal.Kind)),
			slog.String("principal_id", principal.ID))
		message := mcplib.NewPromptMessage(mcplib.RoleUser, mcplib.NewTextContent(skill.Prompt(req.Params.Arguments["ask"])))
		return mcplib.NewGetPromptResult(observePromptDescription, []mcplib.PromptMessage{message}), nil
	}
}
