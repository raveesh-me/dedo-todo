package tasks

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

// Migrations are the module's schema migrations, which the composition root
// applies before modules that depend on this one.
//
//go:embed migrations/*.sql
var migrations embed.FS

// Migrations returns the module's migrations for db.Migrate.
func Migrations() db.Module {
	sub, _ := fs.Sub(migrations, "migrations")
	return db.Module{Name: "tasks", FS: sub}
}

// role is the module's database role: privileges on its own schema only.
const role = "tasks_app"

// errNotFound is a task the caller has not got: none, another person's or
// another location's, or deleted.
var errNotFound = errors.New("task not found")

// errExists is a task ID the caller already uses.
var errExists = errors.New("task already exists")

// store is the only code that reads the tasks schema. Every method runs in
// a transaction InTenantTx scoped to the caller, so row-level security
// keeps each query to the caller's own rows.
type store struct{}

const columns = `id, title, notes, state, deadline_time, create_time, update_time, complete_time, complete_request_id`

func scan(row pgx.Row) (Task, error) {
	var t Task
	var state string
	err := row.Scan(&t.ID, &t.Title, &t.Notes, &state, &t.Deadline, &t.CreateTime, &t.UpdateTime, &t.CompleteTime, &t.CompleteRequestID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, errNotFound
	}
	t.State = State(state)
	t.Deadline = utc(t.Deadline)
	t.CompleteTime = utc(t.CompleteTime)
	t.CreateTime = t.CreateTime.UTC()
	t.UpdateTime = t.UpdateTime.UTC()
	return t, err
}

func (store) insert(ctx context.Context, tx pgx.Tx, ten db.Tenant, t Task) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO tasks.tasks (location_id, actor, id, title, notes, state, deadline_time, create_time, update_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		ten.Location, ten.Actor, t.ID, t.Title, t.Notes, string(t.State), t.Deadline, t.CreateTime, t.UpdateTime)
	if db.IsUniqueViolation(err, "tasks_pkey") {
		return errExists
	}
	return err
}

// get reads a live task, locked for update when lock is set.
func (store) get(ctx context.Context, tx pgx.Tx, id string, lock bool) (Task, error) {
	q := `SELECT ` + columns + ` FROM tasks.tasks WHERE id = $1 AND delete_time IS NULL`
	if lock {
		q += ` FOR UPDATE`
	}
	return scan(tx.QueryRow(ctx, q, id))
}

func (store) save(ctx context.Context, tx pgx.Tx, t Task) error {
	_, err := tx.Exec(ctx, `
		UPDATE tasks.tasks
		SET title = $2, notes = $3, state = $4, deadline_time = $5, update_time = $6,
		    complete_time = $7, complete_request_id = $8
		WHERE id = $1 AND delete_time IS NULL`,
		t.ID, t.Title, t.Notes, string(t.State), t.Deadline, t.UpdateTime, t.CompleteTime, t.CompleteRequestID)
	return err
}

// remove deletes a live task, keeping its tombstone and the request_id
// that deleted it. A task already deleted by the same requestID reports
// replay; any other missing task is errNotFound.
func (store) remove(ctx context.Context, tx pgx.Tx, id, requestID string, now time.Time) (replay bool, err error) {
	tag, err := tx.Exec(ctx, `
		UPDATE tasks.tasks SET delete_time = $2, delete_request_id = $3, update_time = $2
		WHERE id = $1 AND delete_time IS NULL`, id, now, requestID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 1 {
		return false, nil
	}
	if requestID == "" {
		return false, errNotFound
	}
	var same bool
	err = tx.QueryRow(ctx, `
		SELECT delete_request_id = $2 FROM tasks.tasks WHERE id = $1 AND delete_time IS NOT NULL`,
		id, requestID).Scan(&same)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !same) {
		return false, errNotFound
	}
	return err == nil, err
}

// Show is which tasks a list holds.
type show int

const (
	showAll show = iota
	showOpen
	showDone
	showOverdue
)

// list returns up to limit+1 live tasks after the cursor, newest first.
func (store) list(ctx context.Context, tx pgx.Tx, which show, now time.Time, after *page.Key, limit int) ([]Task, error) {
	q := `SELECT ` + columns + ` FROM tasks.tasks WHERE delete_time IS NULL`
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	switch which {
	case showOpen:
		q += ` AND state = 'OPEN'`
	case showDone:
		q += ` AND state = 'DONE'`
	case showOverdue:
		q += ` AND state = 'OPEN' AND deadline_time IS NOT NULL AND deadline_time < ` + arg(now)
	}
	if after != nil {
		q += ` AND (create_time, id) < (` + arg(after.Time) + `, ` + arg(after.ID) + `)`
	}
	q += ` ORDER BY create_time DESC, id DESC LIMIT ` + arg(limit+1)
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
