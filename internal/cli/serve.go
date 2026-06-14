package cli

import (
	"fmt"
	"os"

	"github.com/balpal4495/loar/internal/mcp"
	"github.com/spf13/cobra"
)

// NewServeCmd returns the `loar serve` command.
// It starts a Model Context Protocol (MCP) server over stdio so that AI agents
// (GitHub Copilot, Claude Code) can call `loar_explain` natively via JSON-RPC.
//
// Register in VS Code by adding .vscode/mcp.json:
//
//	{
//	  "servers": {
//	    "loar": {
//	      "type": "stdio",
//	      "command": "loar",
//	      "args": ["serve"]
//	    }
//	  }
//	}
func NewServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start an MCP server for AI agent access",
		Long: `Starts a Model Context Protocol (MCP) server over stdio.

AI agents (GitHub Copilot, Claude Code) connect via VS Code's MCP client and
call the loar_explain tool to retrieve structured evidence from the knowledge
store without leaving the agent context.

The server binds to the project configured in the current directory
(.loar/project.toml). Run from the workspace root so agents discover the
correct project.

To register with VS Code, create .vscode/mcp.json:

  {
    "servers": {
      "loar": {
        "type": "stdio",
        "command": "loar",
        "args": ["serve"]
      }
    }
  }`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			db, cfg, err := openStore(cmd)
			if err != nil {
				return fmt.Errorf("serve: %w", err)
			}
			defer db.Close()

			if err := db.Migrate(ctx); err != nil {
				return fmt.Errorf("serve: migrate: %w", err)
			}

			proj, err := db.GetProjectByName(ctx, cfg.Project)
			if err != nil {
				return fmt.Errorf("serve: project %q not found; run \"loar project use %s\" first", cfg.Project, cfg.Project)
			}

			// Log to stderr so stdout stays clean for JSON-RPC.
			fmt.Fprintf(os.Stderr, "[loar-mcp] serving project %q (id: %s)\n", proj.Name, proj.ID)
			fmt.Fprintf(os.Stderr, "[loar-mcp] tool: loar_explain — ready\n")

			srv := mcp.New(db, proj.ID, proj.Name, cmd.InOrStdin(), cmd.OutOrStdout())
			return srv.Serve(ctx)
		},
	}
}
