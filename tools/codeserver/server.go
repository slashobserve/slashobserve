package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	endpointPath = "/mcp"
	healthPath   = "/healthz"
)

func NewHandler(code *Code, version string) http.Handler {
	srv := server.NewMCPServer("slashobserve-codeserver", version,
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)
	readOnly := []mcp.ToolOption{
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	}
	srv.AddTool(mcp.NewTool("list_dir", append(readOnly,
		mcp.WithDescription("List the entries of a directory in the source tree. Directories end with a slash. Paths are relative to the root; use \".\" for the root."),
		mcp.WithString("path", mcp.Required(), mcp.Description("Directory path relative to the root.")),
	)...), call(func(ctx context.Context, req mcp.CallToolRequest) (string, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return "", refusal("path is required")
		}
		return code.ListDir(ctx, path)
	}))
	srv.AddTool(mcp.NewTool("read_file", append(readOnly,
		mcp.WithDescription("Read a text file from the source tree, with line numbers. Long files are cut; the result says where to continue."),
		mcp.WithString("path", mcp.Required(), mcp.Description("File path relative to the root.")),
		mcp.WithNumber("offset", mcp.Min(1), mcp.Description("First line to return, starting at 1.")),
		mcp.WithNumber("limit", mcp.Min(1), mcp.Description("Most lines to return.")),
	)...), call(func(ctx context.Context, req mcp.CallToolRequest) (string, error) {
		path, err := req.RequireString("path")
		if err != nil {
			return "", refusal("path is required")
		}
		return code.ReadFile(ctx, path, req.GetInt("offset", 1), req.GetInt("limit", maxReadLines))
	}))
	srv.AddTool(mcp.NewTool("grep", append(readOnly,
		mcp.WithDescription("Search file contents with a regular expression (Go syntax; prefix with (?i) to ignore case). Returns path:line: text for each match, and stops with a note after "+strconv.Itoa(maxGrepMatches)+" matches."),
		mcp.WithString("pattern", mcp.Required(), mcp.Description("Regular expression to look for in each line.")),
		mcp.WithString("path", mcp.Description("Directory or file to search, relative to the root. Defaults to the whole root.")),
		mcp.WithString("glob", mcp.Description("Only search files whose name matches, for example *.go; a glob with a slash is matched against the path from the root, and ** matches any number of directories, as in **/*.go.")),
	)...), call(func(ctx context.Context, req mcp.CallToolRequest) (string, error) {
		pattern, err := req.RequireString("pattern")
		if err != nil {
			return "", refusal("pattern is required")
		}
		return code.Grep(ctx, pattern, req.GetString("path", "."), req.GetString("glob", ""))
	}))

	streamable := server.NewStreamableHTTPServer(srv, server.WithEndpointPath(endpointPath), server.WithStateLess(true))
	mux := http.NewServeMux()
	mux.Handle(endpointPath, streamable)
	mux.HandleFunc(healthPath, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

func call(fn func(context.Context, mcp.CallToolRequest) (string, error)) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := fn(ctx, req)
		if err == nil {
			return mcp.NewToolResultText(text), nil
		}
		var r refusal
		if errors.As(err, &r) {
			return mcp.NewToolResultError(err.Error()), nil
		}
		slog.Error("tool call failed", "tool", req.Params.Name, "cause", err.Error())
		return mcp.NewToolResultError("the server could not complete the call"), nil
	}
}
