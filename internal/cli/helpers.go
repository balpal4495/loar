// Package cli assembles the loar command tree and shared configuration.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// databaseDSN is read from the LOAR_DATABASE_URL environment variable.
func databaseDSN() string {
	return os.Getenv("LOAR_DATABASE_URL")
}

// mustDSN returns the DSN or prints an error and exits.
func mustDSN(cmd *cobra.Command) string {
	dsn := databaseDSN()
	if dsn == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "error: LOAR_DATABASE_URL environment variable is not set")
		os.Exit(1)
	}
	return dsn
}
