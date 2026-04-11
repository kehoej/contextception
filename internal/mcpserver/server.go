// Package mcpserver exposes contextception via the Model Context Protocol.
package mcpserver

import (
	"context"
	"io"
	"sync"

	"github.com/kehoej/contextception/internal/config"
	"github.com/kehoej/contextception/internal/db"
	"github.com/kehoej/contextception/internal/history"
	"github.com/kehoej/contextception/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ContextServer wraps the MCP server with repo-specific state.
type ContextServer struct {
	repoRoot string

	mu   sync.Mutex
	idx  *db.Index       // lazily opened on first tool call
	cfg  *config.Config  // lazily loaded on first tool call
	hist *history.Store  // lazily opened on first usage-tracking call
}

// New creates a ContextServer for the given repository root.
func New(repoRoot string) *ContextServer {
	return &ContextServer{repoRoot: repoRoot}
}

// Run starts the MCP server on the given transport (typically stdio).
// It blocks until the client disconnects or the context is cancelled.
func (cs *ContextServer) Run(ctx context.Context, t mcp.Transport) error {
	defer cs.Close()

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "contextception",
			Version: version.Version,
		},
		nil,
	)

	cs.registerTools(server)

	return server.Run(ctx, t)
}

// RunStdio starts the MCP server on stdin/stdout.
func (cs *ContextServer) RunStdio(ctx context.Context) error {
	return cs.Run(ctx, &mcp.StdioTransport{})
}

// Close releases the database connections if open.
func (cs *ContextServer) Close() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.idx != nil {
		cs.idx.Close()
		cs.idx = nil
	}
	if cs.hist != nil {
		cs.hist.Close()
		cs.hist = nil
	}
}

// ensureIndex opens the index database, creating and migrating it if needed.
func (cs *ContextServer) ensureIndex() (*db.Index, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.idx != nil {
		return cs.idx, nil
	}

	idxPath := db.IndexPath(cs.repoRoot)
	idx, err := db.OpenIndex(idxPath)
	if err != nil {
		return nil, err
	}

	applied, err := idx.MigrateToLatest()
	if err != nil {
		idx.Close()
		return nil, err
	}

	if applied {
		_ = idx.SetMeta("last_indexed_commit", "")
	}

	cs.idx = idx
	return cs.idx, nil
}

// ensureConfig lazily loads the repository config.
func (cs *ContextServer) ensureConfig() *config.Config {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.cfg != nil {
		return cs.cfg
	}

	cfg, err := config.Load(cs.repoRoot)
	if err != nil {
		// Config loading failure is non-fatal; fall back to empty.
		cs.cfg = config.Empty()
		return cs.cfg
	}

	if cfg.Version > 0 {
		if _, err := cfg.Validate(cs.repoRoot); err != nil {
			cs.cfg = config.Empty()
			return cs.cfg
		}
	}

	cs.cfg = cfg
	return cs.cfg
}

// ensureHistory lazily opens the history store for usage tracking.
func (cs *ContextServer) ensureHistory() *history.Store {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.hist != nil {
		return cs.hist
	}

	hist, err := history.Open(cs.repoRoot)
	if err != nil {
		return nil
	}

	cs.hist = hist
	return cs.hist
}

// registerTools adds all MCP tools to the server.
func (cs *ContextServer) registerTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_context",
		Description: "Analyze a file's context dependencies. Returns categorized lists of files that must be understood before making a safe change. Auto-indexes the repository if needed. Accepts a single file path or an array of file paths for multi-file analysis.",
	}, cs.handleGetContext)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "index",
		Description: "Build or update the contextception index for the repository. Uses incremental indexing when possible.",
	}, cs.handleIndex)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "status",
		Description: "Return index diagnostics: file count, edge count, staleness, and last indexed commit.",
	}, cs.handleStatus)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search the index for files by path pattern or symbol name. Returns matching files with structural signals (indegree, role). Use type='symbol' to find files that export a specific name.",
	}, cs.handleSearch)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_entrypoints",
		Description: "Return the project's entrypoint files (main modules, CLI entry points) and foundation files (most depended-upon). Use for initial project orientation.",
	}, cs.handleGetEntrypoints)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_structure",
		Description: "Return directory structure with file counts and language distribution. Use as the first call when exploring an unfamiliar project.",
	}, cs.handleGetStructure)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_archetypes",
		Description: "Detect representative files across architectural layers. Returns one file per archetype category, selected using structural and git signals. Use for initial codebase orientation. Optionally filter to specific categories via the 'categories' parameter. Available categories: Service/Controller, Model/Schema, Middleware/Plugin, High Fan-in Utility, Page/Route/Endpoint, Auth/Security, Leaf Component, Config/Constants, Barrel/Index, Test File, Database/Migration, Serialization/Validation, Error Handling, CLI/Command, Event/Message, Interface/Contract, Orchestrator, Hotspot.",
	}, cs.handleGetArchetypes)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "analyze_change",
		Description: "Analyze the impact of a git diff (PR or branch). Returns changed files with blast radius, aggregated must-read context, test gaps, coupling signals, and hotspots. If base is omitted, auto-detects merge-base against the default branch (main/master).",
	}, cs.handleAnalyzeChange)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "rate_context",
		Description: "Rate how useful the last get_context result was for your work. Call this after completing work on a file you previously analyzed. Reports which suggested files were useful, which were unnecessary, and which needed files were missing. Helps improve recommendation quality over time.",
	}, cs.handleRateContext)
}

// logWriter returns an io.Writer that discards output (MCP servers must not write to stdout).
func logWriter() io.Writer {
	return io.Discard
}
