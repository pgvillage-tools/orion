package helpers

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// PgConnParams are the parameters of a postgres connection, e.g. host, port and user
type PgConnParams map[string]string

// ConnString returns the parameters as a connection string (key='value' ...)
func (cp PgConnParams) ConnString() string {
	var connString []string
	for k, v := range cp {
		v = fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", `\'`))
		item := fmt.Sprintf("%s=%s", k, v)
		connString = append(connString, item)
	}
	return strings.Join(connString, " ")
}

// SetParam returns a copy of the parameters with key set to value
func (cp PgConnParams) SetParam(key, value string) PgConnParams {
	newCp := maps.Clone(cp)
	newCp[key] = value
	return newCp
}

// PgPing connects to postgres and runs a query
func PgPing(ctx context.Context, connParams PgConnParams) error {
	conn, err := pgx.Connect(ctx, connParams.ConnString())
	if err != nil {
		return err
	}
	defer conn.Close(ctx)

	var name string
	err = conn.QueryRow(ctx, "select datname from pg_database;").Scan(&name)
	if err != nil {
		return err
	}
	return nil
}

// IsReady pings postgres every interval, until it answers (nil) or ctx is done (the error of the last ping)
func IsReady(ctx context.Context, connParams PgConnParams, interval time.Duration) error {
	for {
		err := PgPing(ctx, connParams)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("postgres not ready: %w", err)
		case <-time.After(interval):
		}
	}
}

// PgExec connects to postgres and runs sql (e.g. a create table or an insert)
func PgExec(ctx context.Context, connParams PgConnParams, sql string, args ...any) error {
	conn, err := pgx.Connect(ctx, connParams.ConnString())
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, sql, args...)
	return err
}

// PgQueryInt connects to postgres and returns the integer that query returns (e.g. a count)
func PgQueryInt(ctx context.Context, connParams PgConnParams, query string, args ...any) (int, error) {
	conn, err := pgx.Connect(ctx, connParams.ConnString())
	if err != nil {
		return 0, err
	}
	defer conn.Close(ctx)
	var n int
	err = conn.QueryRow(ctx, query, args...).Scan(&n)
	return n, err
}
