package time_tracking

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/raveesh-me/dedo-todo/internal/shared/db"
	"github.com/raveesh-me/dedo-todo/internal/shared/page"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrations returns the module's migrations for db.Migrate.
func Migrations() db.Module {
	sub, _ := fs.Sub(migrations, "migrations")
	return db.Module{Name: "time_tracking", FS: sub}
}

// role is the module's database role: privileges on its own schema only.
const role = "time_tracking_app"

var (
	errNotFound       = errors.New("not found")
	errExists         = errors.New("already exists")
	errAlreadyRunning = errors.New("a timer is already running")
)

// store is the only code that reads the time_tracking schema. Every method
// runs in a transaction InTenantTx scoped to the caller.
type store struct{}

func (store) startTimer(ctx context.Context, tx pgx.Tx, ten db.Tenant, t Timer) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO time_tracking.timers (location_id, actor, id, task, start_time)
		VALUES ($1, $2, $3, $4, $5)`, ten.Location, ten.Actor, t.ID, t.Task, t.StartTime)
	if db.IsUniqueViolation(err, "timers_one_per_person") {
		return errAlreadyRunning
	}
	return err
}

func (store) runningTimer(ctx context.Context, tx pgx.Tx) (Timer, error) {
	return scanTimer(tx.QueryRow(ctx, `SELECT id, task, start_time FROM time_tracking.timers`))
}

func (store) getTimer(ctx context.Context, tx pgx.Tx, id string) (Timer, error) {
	return scanTimer(tx.QueryRow(ctx, `SELECT id, task, start_time FROM time_tracking.timers WHERE id = $1`, id))
}

// takeTimer removes a running timer and returns it: whoever takes it writes
// its one time entry.
func (store) takeTimer(ctx context.Context, tx pgx.Tx, id string) (Timer, error) {
	return scanTimer(tx.QueryRow(ctx, `DELETE FROM time_tracking.timers WHERE id = $1 RETURNING id, task, start_time`, id))
}

func scanTimer(row pgx.Row) (Timer, error) {
	var t Timer
	err := row.Scan(&t.ID, &t.Task, &t.StartTime)
	if errors.Is(err, pgx.ErrNoRows) {
		return Timer{}, errNotFound
	}
	t.StartTime = t.StartTime.UTC()
	return t, err
}

func (store) insertEntry(ctx context.Context, tx pgx.Tx, ten db.Tenant, e TimeEntry) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO time_tracking.time_entries (location_id, actor, id, task, start_time, end_time, origin, create_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		ten.Location, ten.Actor, e.ID, e.Task, e.StartTime, e.EndTime, string(e.Origin), e.CreateTime)
	if db.IsUniqueViolation(err, "time_entries_pkey") {
		return errExists
	}
	return err
}

const entryColumns = `id, task, start_time, end_time, origin, create_time`

func scanEntry(row pgx.Row) (TimeEntry, error) {
	var e TimeEntry
	var origin string
	err := row.Scan(&e.ID, &e.Task, &e.StartTime, &e.EndTime, &origin, &e.CreateTime)
	if errors.Is(err, pgx.ErrNoRows) {
		return TimeEntry{}, errNotFound
	}
	e.Origin = Origin(origin)
	e.StartTime, e.EndTime, e.CreateTime = e.StartTime.UTC(), e.EndTime.UTC(), e.CreateTime.UTC()
	return e, err
}

func (store) getEntry(ctx context.Context, tx pgx.Tx, id string) (TimeEntry, error) {
	return scanEntry(tx.QueryRow(ctx, `SELECT `+entryColumns+` FROM time_tracking.time_entries WHERE id = $1`, id))
}

func (store) deleteEntry(ctx context.Context, tx pgx.Tx, id string) error {
	tag, err := tx.Exec(ctx, `DELETE FROM time_tracking.time_entries WHERE id = $1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return errNotFound
	}
	return err
}

// listEntries returns up to limit+1 entries after the cursor, latest start
// first, only task's when task is set.
func (store) listEntries(ctx context.Context, tx pgx.Tx, task string, after *page.Key, limit int) ([]TimeEntry, error) {
	q := `SELECT ` + entryColumns + ` FROM time_tracking.time_entries WHERE true`
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	if task != "" {
		q += ` AND task = ` + arg(task)
	}
	if after != nil {
		q += ` AND (start_time, id) < (` + arg(after.Time) + `, ` + arg(after.ID) + `)`
	}
	q += ` ORDER BY start_time DESC, id DESC LIMIT ` + arg(limit+1)
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TimeEntry
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// trackedTime sums a task's time entries.
func (store) trackedTime(ctx context.Context, tx pgx.Tx, task string) (time.Duration, int, error) {
	var micros int64
	var n int
	err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(EXTRACT(EPOCH FROM (end_time - start_time)) * 1000000), 0)::bigint, COUNT(*)
		FROM time_tracking.time_entries WHERE task = $1`, task).Scan(&micros, &n)
	return time.Duration(micros) * time.Microsecond, n, err
}
