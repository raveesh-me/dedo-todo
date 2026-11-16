// Package tasks is the Tasks module (commerce.todo.tasks.v1): the Task
// aggregate, its rules, its store and the handlers of TaskService. It knows
// nothing of time tracking: it announces what happened to a task as events,
// outward, and whoever reacts calls the API.
package tasks

import (
	"errors"
	"time"
)

// State is whether a task is still to be done.
type State string

// A task's two states.
const (
	Open State = "OPEN"
	Done State = "DONE"
)

// The rules a task keeps. Handlers turn each into its declared reason.
var (
	ErrTitleRequired   = errors.New("a task needs a title")
	ErrDeadlineInPast  = errors.New("a deadline cannot be in the past")
	ErrAlreadyDone     = errors.New("the task is already done")
	ErrNotDone         = errors.New("the task is not done")
	ErrDoneImmutable   = errors.New("a done task's deadline cannot change")
	ErrTitleTooLong    = errors.New("a title has at most 200 characters")
	errUnknownMaskPath = errors.New("unknown field in update_mask")
)

// MaxTitle is the longest title, in characters.
const MaxTitle = 200

// Task is the aggregate root: something a person intends to do.
type Task struct {
	ID           string
	Title        string
	Notes        string
	State        State
	Deadline     *time.Time // nil: no deadline, never overdue
	CreateTime   time.Time
	UpdateTime   time.Time
	CompleteTime *time.Time
	// CompleteRequestID is the request_id of the CompleteTask call that
	// completed the task, so a retry of that call succeeds again.
	CompleteRequestID string
}

// New returns a new open task. A deadline must not be in the past at now.
func New(id, title, notes string, deadline *time.Time, now time.Time) (Task, error) {
	if err := checkTitle(title); err != nil {
		return Task{}, err
	}
	if deadline != nil && deadline.Before(now) {
		return Task{}, ErrDeadlineInPast
	}
	return Task{
		ID:         id,
		Title:      title,
		Notes:      notes,
		State:      Open,
		Deadline:   utc(deadline),
		CreateTime: now,
		UpdateTime: now,
	}, nil
}

// Overdue reports whether the task is open and its deadline has passed at
// now. Derived on every read, never stored.
func (t Task) Overdue(now time.Time) bool {
	return t.State == Open && t.Deadline != nil && t.Deadline.Before(now)
}

// Complete moves an open task to done. A retry of the call that completed
// it, with the same non-empty requestID, is not an error: replay reports
// it, so the caller announces the completion again.
func (t *Task) Complete(requestID string, now time.Time) (replay bool, err error) {
	if t.State == Done {
		if requestID != "" && requestID == t.CompleteRequestID {
			return true, nil
		}
		return false, ErrAlreadyDone
	}
	t.State = Done
	t.CompleteTime = &now
	t.CompleteRequestID = requestID
	t.UpdateTime = now
	return false, nil
}

// Reopen moves a done task back to open, keeping its deadline.
func (t *Task) Reopen(now time.Time) error {
	if t.State != Done {
		return ErrNotDone
	}
	t.State = Open
	t.CompleteTime = nil
	t.CompleteRequestID = ""
	t.UpdateTime = now
	return nil
}

// Change is an update to a task: the fields named are set, the rest kept.
type Change struct {
	Title       *string
	Notes       *string
	SetDeadline bool       // the deadline is named
	Deadline    *time.Time // its new value; nil clears it
}

// Apply changes the task. A deadline can be set, moved or cleared only on
// an open task, and never to a time in the past.
func (t *Task) Apply(c Change, now time.Time) error {
	if c.Title != nil {
		if err := checkTitle(*c.Title); err != nil {
			return err
		}
	}
	if c.SetDeadline {
		if t.State == Done {
			return ErrDoneImmutable
		}
		if c.Deadline != nil && c.Deadline.Before(now) {
			return ErrDeadlineInPast
		}
	}
	if c.Title != nil {
		t.Title = *c.Title
	}
	if c.Notes != nil {
		t.Notes = *c.Notes
	}
	if c.SetDeadline {
		t.Deadline = utc(c.Deadline)
	}
	t.UpdateTime = now
	return nil
}

func checkTitle(title string) error {
	n := len([]rune(title))
	if n == 0 {
		return ErrTitleRequired
	}
	if n > MaxTitle {
		return ErrTitleTooLong
	}
	return nil
}

func utc(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
