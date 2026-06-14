package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// CreateDatabase creates a Postgres database named dbName owned by owner.
// adminDSN must connect to a database where the user has CREATEDB privileges.
// A "database already exists" error is silently ignored.
func CreateDatabase(ctx context.Context, adminDSN, dbName, owner string) error {
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return fmt.Errorf("admin: connect: %w", err)
	}
	defer conn.Close(ctx)

	dbIdent := pgx.Identifier{dbName}.Sanitize()
	ownerIdent := pgx.Identifier{owner}.Sanitize()

	_, err = conn.Exec(ctx, fmt.Sprintf(`CREATE DATABASE %s WITH OWNER %s`, dbIdent, ownerIdent))
	if err != nil && !isDuplicateDatabase(err) {
		return fmt.Errorf("admin: create database %q: %w", dbName, err)
	}
	return nil
}

// DropDatabase terminates all connections to dbName and then drops it.
// adminDSN must connect as a superuser or the database owner.
func DropDatabase(ctx context.Context, adminDSN, dbName string) error {
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return fmt.Errorf("admin: connect: %w", err)
	}
	defer conn.Close(ctx)

	// Terminate any open connections so DROP DATABASE does not block.
	_, _ = conn.Exec(ctx, fmt.Sprintf(
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid()`,
		strings.ReplaceAll(dbName, "'", "''"),
	))

	dbIdent := pgx.Identifier{dbName}.Sanitize()
	_, err = conn.Exec(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS %s`, dbIdent))
	if err != nil {
		return fmt.Errorf("admin: drop database %q: %w", dbName, err)
	}
	return nil
}

// EnsureUser creates a Postgres login role with the given password if it does
// not already exist. adminDSN must connect as a superuser.
func EnsureUser(ctx context.Context, adminDSN, user, password string) error {
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return fmt.Errorf("admin: connect: %w", err)
	}
	defer conn.Close(ctx)

	var exists bool
	if err := conn.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname = $1)`, user,
	).Scan(&exists); err != nil {
		return fmt.Errorf("admin: check user %q: %w", user, err)
	}

	userIdent := pgx.Identifier{user}.Sanitize()
	safePassword := strings.ReplaceAll(password, "'", "''")

	if exists {
		// Ensure CREATEDB is granted even if the user pre-existed without it.
		_, err = conn.Exec(ctx, fmt.Sprintf(`ALTER USER %s WITH CREATEDB`, userIdent))
		return err
	}

	_, err = conn.Exec(ctx, fmt.Sprintf(`CREATE USER %s WITH PASSWORD '%s' CREATEDB`, userIdent, safePassword))
	if err != nil {
		return fmt.Errorf("admin: create user %q: %w", user, err)
	}
	return nil
}

// ListLoarDatabases returns the project names (the part after "loar-") of all
// databases on the server whose names start with "loar-".
func ListLoarDatabases(ctx context.Context, adminDSN string) ([]string, error) {
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return nil, fmt.Errorf("admin: connect: %w", err)
	}
	defer conn.Close(ctx)

	rows, err := conn.Query(ctx,
		`SELECT datname FROM pg_database WHERE datname LIKE 'loar-%' ORDER BY datname`)
	if err != nil {
		return nil, fmt.Errorf("admin: list databases: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, strings.TrimPrefix(name, "loar-"))
	}
	return names, rows.Err()
}

// isDuplicateDatabase reports whether err is a Postgres "duplicate database"
// error (SQLSTATE 42P04).
func isDuplicateDatabase(err error) bool {
	return err != nil && strings.Contains(err.Error(), "42P04")
}
