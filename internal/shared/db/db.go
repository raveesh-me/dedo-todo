// Package db is the service's one way into Postgres. Every read and write a
// handler makes runs in InTenantTx: one transaction that first sets the
// caller's location and actor, which each table's row-level security policy
// checks, and switches to the module's own role, which owns no tables. A
// query that forgets its WHERE still sees only the caller's rows, and a
// module that reaches into another module's schema fails: its role has no
// privileges there.
package db

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/GoHighLevel/commerce-auth/go/iamguard"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Open connects to the database at url, as the role that owns the schemas:
// it runs the migrations, and InTenantTx switches each transaction to a
// module's role.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	if url == "" {
		return nil, errors.New("DATABASE_URL is not set: the Postgres URL, such as postgres://postgres:postgres@localhost:15432/todo?sslmode=disable")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: %w", err)
	}
	return pool, nil
}

// Tenant is whose rows a transaction may see: the caller's location, from
// the token IAM verified, and the person (actor) acting in it.
type Tenant struct {
	Location string
	Actor    string
}

// ErrNoTenant is a call that reached storage with no verified caller. The
// guard refuses such calls first; this is the last line, never run
// unscoped.
var ErrNoTenant = errors.New("db: no verified tenant in the call's context")

// TenantFrom returns the tenant of the call in ctx: the location IAM
// verified the call against, and the token it verified as the actor.
func TenantFrom(ctx context.Context) (Tenant, error) {
	caller, ok := iamguard.FromContext(ctx)
	if !ok || caller.Target.Class != "Location" || caller.Target.ID == "" || caller.Principal.AuthClassID == "" {
		return Tenant{}, ErrNoTenant
	}
	return Tenant{
		Location: caller.Target.ID,
		Actor:    caller.Principal.AuthClass + ":" + caller.Principal.AuthClassID,
	}, nil
}

var roleName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// InTenantTx runs fn in one transaction as role, scoped to the tenant of the
// call in ctx, and commits when fn returns nil.
func InTenantTx(ctx context.Context, pool *pgxpool.Pool, role string, fn func(pgx.Tx, Tenant) error) error {
	t, err := TenantFrom(ctx)
	if err != nil {
		return err
	}
	if !roleName.MatchString(role) {
		return fmt.Errorf("db: bad role %q", role)
	}
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		// set_config(..., true) is SET LOCAL: all three end with the
		// transaction, so a pooled connection never carries a tenant or a
		// role to the next caller.
		if _, err := tx.Exec(ctx,
			"SELECT set_config('app.location_id', $1, true), set_config('app.actor', $2, true), set_config('role', $3, true)",
			t.Location, t.Actor, role); err != nil {
			return fmt.Errorf("db: scope transaction: %w", err)
		}
		return fn(tx, t)
	})
}

// IsUniqueViolation reports whether err is a unique constraint's.
func IsUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && (constraint == "" || pgErr.ConstraintName == constraint)
}
