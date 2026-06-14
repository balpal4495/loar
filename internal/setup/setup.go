// Package setup provides Postgres detection utilities used by `loar setup`.
package setup

import (
	"context"
	"net"
	"os/exec"
	"runtime"
	"time"
)

// PostgresStatus describes the local Postgres state.
type PostgresStatus int

const (
	// PostgresRunning means Postgres is accepting connections on localhost:5432.
	PostgresRunning PostgresStatus = iota
	// PostgresNotFound means no Postgres process was found.
	PostgresNotFound
)

// DetectPostgres checks whether Postgres is reachable on localhost:5432.
func DetectPostgres() PostgresStatus {
	conn, err := net.DialTimeout("tcp", "localhost:5432", 2*time.Second)
	if err != nil {
		return PostgresNotFound
	}
	conn.Close()
	return PostgresRunning
}

// DockerAvailable reports whether the Docker daemon is reachable.
func DockerAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}

// IsMacOS reports whether we are running on macOS.
func IsMacOS() bool {
	return runtime.GOOS == "darwin"
}

// StartInstructions returns the shell commands to start an already-installed
// Postgres instance.
func StartInstructions() string {
	if IsMacOS() {
		return "  brew services start postgresql@16"
	}
	return "  sudo systemctl start postgresql"
}

// InstallInstructions returns the shell commands to install and start Postgres.
func InstallInstructions() string {
	if IsMacOS() {
		return "  brew install postgresql@16 && brew services start postgresql@16"
	}
	return "  # See https://www.postgresql.org/download/ for your Linux distribution"
}
