// Package time_tracking is the Time Tracking module
// (commerce.todo.time_tracking.v1): the Timer aggregate, at most one per
// person; the immutable TimeEntry records a stopped timer or a person
// writes; and the handlers of TimerService and TimeEntryService.
//
// It never reads the Tasks module's tables or Go packages. What it needs to
// know about a task it asks through Tasks, a port of its own that the
// composition root implements over Tasks' published contract
// (commerce.todo.tasks.v1.TaskService.GetTask), forwarding the caller's
// token: an anti-corruption layer that keeps Tasks' model out of this one.
package time_tracking

import (
	"context"
	"errors"
	"time"
)

// Tasks is what Time Tracking asks about a task, in its own terms.
type Tasks interface {
	// Open reports whether the caller's task named name (tasks/{task}) is
	// open. It returns ErrTaskNotFound when the caller has no such task,
	// and ErrTasksUnavailable when Tasks could not answer.
	Open(ctx context.Context, name string) (bool, error)
}

// Errors a Tasks implementation returns.
var (
	ErrTaskNotFound     = errors.New("task not found")
	ErrTasksUnavailable = errors.New("tasks unavailable")
)

// Origin is how a time entry was recorded.
type Origin string

// The two origins of a time entry.
const (
	FromTimer Origin = "TIMER"
	Manual    Origin = "MANUAL"
)

// The rules time tracking keeps.
var (
	ErrInvalidInterval = errors.New("a time entry ends after it starts")
)

// Timer is the aggregate root: a running measurement of a person's time on
// one task. A person has at most one; the store's unique key on the
// person keeps that true under concurrent starts.
type Timer struct {
	ID        string
	Task      string // tasks/{task}
	StartTime time.Time
}

// TimeEntry is a finished, immutable record of time spent on a task.
type TimeEntry struct {
	ID         string
	Task       string
	StartTime  time.Time
	EndTime    time.Time
	Origin     Origin
	CreateTime time.Time
}

// Duration is how long the entry lasted.
func (e TimeEntry) Duration() time.Duration { return e.EndTime.Sub(e.StartTime) }

// Stop ends the timer at now and returns the one time entry it writes,
// with id. A clock that went backwards gives a zero-length entry, never a
// negative one.
func (t Timer) Stop(id string, now time.Time) TimeEntry {
	end := now
	if end.Before(t.StartTime) {
		end = t.StartTime
	}
	return TimeEntry{ID: id, Task: t.Task, StartTime: t.StartTime, EndTime: end, Origin: FromTimer, CreateTime: now}
}

// NewManualEntry returns a time entry a person recorded by hand. Its end
// must be after its start.
func NewManualEntry(id, task string, start, end, now time.Time) (TimeEntry, error) {
	if !end.After(start) {
		return TimeEntry{}, ErrInvalidInterval
	}
	return TimeEntry{ID: id, Task: task, StartTime: start.UTC(), EndTime: end.UTC(), Origin: Manual, CreateTime: now}, nil
}
