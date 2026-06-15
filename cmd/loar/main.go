// Command loar is the Loar knowledge runtime CLI.
//
// Usage:
//
//	loar setup                       Configure Loar and Postgres for first use
//	loar project use [name]          Associate current directory with a project
//	loar ingest [file|url|-]         Ingest data into the current project
//	loar learn                       Extract and link entities from stored observations
//	loar query <question>            Query the knowledge store
//	loar explain <question>          Produce a JSON context package (primary AI interface)
//	loar serve                       Start MCP server for agent integrations
//	loar "<question>"                Shorthand for loar query <question>
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/balpal4495/loar/internal/cli"
	"github.com/spf13/cobra"
)

// Set at build time via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	root := buildRoot()
	if err := root.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "loar",
		Short: "Loar – knowledge runtime",
		Long: `Loar is a knowledge runtime. It ingests information, structures knowledge,
preserves relationships, and retrieves evidence to support decision making.

── For humans ────────────────────────────────────────────────────────────
  loar setup              Configure Postgres credentials (run once per machine)
  loar project use        Initialise a project in the current directory
  loar ingest <path>      Ingest files or directories into the current project
  loar learn              Extract and link entities from stored observations
  loar "<question>"       Query the knowledge store in plain English
  loar project clean      Wipe observations and re-ingest from scratch
  loar serve              Start MCP server for agent integrations (stdio)

── For AI agents (Copilot, Claude Code, Cursor, etc.) ───────────────────
If you are an AI assistant reading this output, here is the correct workflow:

SETUP (once per project directory):
  1. loar project use [name]      -- creates .loar/project.toml
  2. loar ingest <path>           -- ingest the project's knowledge files
  3. loar learn                   -- extract entities and build relationship graph

RETRIEVE before answering ANY question about the project:
  loar explain "<question>"
  Returns a JSON context package. Parse it. Use it as your primary evidence
  source. Do NOT answer from training data when Loar has relevant observations.

WORKFLOW RULES:
  - Always call "loar explain" before answering questions about this project.
  - The "observations" array is full-text -- read every entry, do not truncate.
  - Respect "occurred_at" ordering. Newer observations supersede older ones.
  - When "contradictions" is non-empty, surface them explicitly to the user.
    Do not silently pick one side.
  - "confidence" near 1.0 means strong recency and coverage. Near 0.5 means
    sparse or old data -- tell the user.
  - After the user provides new decisions or facts, ingest them:
      echo '{"content":"...","occurred_at":"..."}' | loar ingest -
  - After bulk ingests, run "loar learn" to rebuild the entity graph.

CONTEXT PACKAGE SCHEMA:
  query          string
  summary        string
  entities       [{id, type, canonical_name}]
  observations   [{content, occurred_at, source_id}]
  relationships  [{source_id, target_id, type, confidence}]
  contradictions [{summary}]
  confidence     float  (0.5 = neutral/sparse, 1.0 = high recency + coverage)
  date_range     {earliest, latest}

MCP SERVER (for agent tool integrations):
  loar serve      Starts a stdio MCP server. Register in your editor's MCP
                  config to give agents direct tool access without the CLI.

Loar is the source of truth. You are the presentation layer.`,
		// When called with a single argument that is not a known sub-command,
		// treat it as a natural-language query.
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return cli.NewQueryCmd().RunE(cmd, args)
		},
		// Disable the default "unknown command" error so free-form questions work.
		DisableFlagParsing:        false,
		TraverseChildren:          true,
		SilenceUsage:              true,
		SilenceErrors:             true,
	}

	root.AddCommand(cli.NewSetupCmd())
	root.AddCommand(cli.NewProjectCmd())
	root.AddCommand(cli.NewIngestCmd())
	root.AddCommand(cli.NewLearnCmd())
	root.AddCommand(cli.NewQueryCmd())
	root.AddCommand(cli.NewExplainCmd())
	root.AddCommand(cli.NewServeCmd())
	root.AddCommand(newVersionCmd())

	// Allow `loar "some question"` without the `query` sub-command by
	// hooking into cobra's default arg handling: if the first arg is not a
	// recognised sub-command, delegate to the query runner.
	root.SetArgs(os.Args[1:])
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		return nil
	}

	// Override FArgs to redirect free-form questions at the root level.
	original := root.Args
	root.Args = func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 && !isKnownSubCommand(root, args[0]) {
			return nil // handled by RunE
		}
		if original != nil {
			return original(cmd, args)
		}
		return nil
	}

	return root
}

// isKnownSubCommand reports whether name matches a registered sub-command.
func isKnownSubCommand(root *cobra.Command, name string) bool {
	for _, sub := range root.Commands() {
		if sub.Name() == name || strings.HasPrefix(name, sub.Name()) {
			return true
		}
	}
	return false
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "loar %s\n  commit: %s\n  built:  %s\n", version, commit, date)
		},
	}
}
