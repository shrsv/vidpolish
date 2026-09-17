// Package mcpserver exposes vidpolish's pipeline, project/cell notebook
// model, YouTube upload, and config/cache/tool-status surfaces as an MCP
// (Model Context Protocol) server over stdio, so an MCP client such as
// Claude can drive vidpolish directly instead of shelling out to the CLI
// or scraping the web UI.
//
// Tool handlers call directly into internal/pipeline, internal/store,
// internal/ytupload, internal/ytauth, and internal/config — the same
// packages cmd/vidpolish/main.go and internal/server call — rather than
// going through the local HTTP API, so this works headless without
// `vidpolish ui` running.
package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"vidpolish/internal/store"
)

// Server holds the shared state behind the MCP tool handlers.
type Server struct {
	db      *store.DB
	version string
	jobs    *jobTracker
}

// Run builds a vidpolish MCP server backed by db and serves it on stdio
// until ctx is cancelled or the transport closes.
func Run(ctx context.Context, db *store.DB, version string) error {
	s := &Server{db: db, version: version, jobs: newJobTracker()}
	srv := mcp.NewServer(&mcp.Implementation{Name: "vidpolish", Version: version}, nil)
	s.registerTools(srv)
	return srv.Run(ctx, &mcp.StdioTransport{})
}

func (s *Server) registerTools(srv *mcp.Server) {
	s.addPipelineTools(srv)
	s.addProjectTools(srv)
	s.addProfileTools(srv)
	s.addYouTubeTools(srv)
	s.addConfigTools(srv)
}

// progressLogger returns a Log-style callback (the shape pipeline.Options
// and ytupload.Options expect) that forwards each line as an MCP progress
// notification instead of the packages' stdout fallback. On the stdio
// transport, stdout is the JSON-RPC channel itself, so a tool handler must
// never leave Log unset — that would otherwise interleave plain-text
// progress lines with protocol frames and corrupt the stream.
func progressLogger(ctx context.Context, req *mcp.CallToolRequest) func(string) {
	token := req.Params.GetProgressToken()
	n := 0.0
	return func(msg string) {
		if token == nil || req.Session == nil {
			return
		}
		n++
		req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
			ProgressToken: token,
			Message:       msg,
			Progress:      n,
		})
	}
}

// toolError formats an error as a CallToolResult with IsError set, the
// SDK's convention for reporting a failed tool call to the model without
// making it a transport-level error.
func toolError(err error) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}, nil, nil
}

func errf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
