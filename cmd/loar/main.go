// Command loar is the Loar knowledge runtime CLI.
//
// Usage:
//
//	loar setup                       Configure Loar and Postgres for first use
//	loar project use [name]          Associate current directory with a project
//	loar ingest [file|url|-]         Ingest data into the current project
//	loar learn                       Extract and link entities from stored observations
//	loar query <question>            Query the knowledge store
//	loar explain <question>          Produce a human-readable explanation
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

── For AI agents (Copilot, Claude Code, etc.) ────────────────────────────
If you are an AI assistant reading this, here is how to use Loar:

1. RETRIEVE context before answering any question about the project:
     loar explain "<question>"
   This returns a JSON context package containing observations, entities,
   contradictions, and a date range. Use this as your primary evidence source.

2. SYNTHESIZE the JSON into a clear answer for the user. Loar does not
   generate prose — that is your job. Structure your answer around the
   observations returned, respecting their temporal order (oldest first).

3. SURFACE contradictions when present. If the context package contains
   contradictions, flag them explicitly rather than silently picking one side.

4. INGEST new knowledge when the user provides information worth preserving:
     echo '<json>' | loar ingest -
   or
     loar ingest <file>

5. LEARN after bulk ingests to extract entity links:
     loar learn

The context package schema:
  query          string
  entities       [{name, type}]
  observations   [{content, occurred_at, source_id}]   -- full text, no truncation
  contradictions [{summary}]
  confidence     float (0–1)
  date_range     {earliest, latest}

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
