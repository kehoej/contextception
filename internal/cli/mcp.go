package cli

import (
	"github.com/kehoej/contextception/internal/mcpserver"
	"github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the MCP server (stdio transport)",
		Long:  "Starts a Model Context Protocol server over stdin/stdout for integration with AI tools like Claude Code.",
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := mcpserver.New(repoRoot)
			return srv.RunStdio(cmd.Context())
		},
	}
}
