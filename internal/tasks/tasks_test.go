package tasks

import (
	"errors"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func TestNewTaskIsOpen(t *testing.T) {
	task, err := New("a", "Write the report", "", nil, t0)
	if err != nil {
		t.Fatal(err)
	}
	if task.State != Open {
		t.Fatalf("state = %s, want OPEN", task.State)
	}
}

func TestNewTaskTitle(t *testing.T) {
	if _, err := New("a", "", "", nil, t0); !errors.Is(err, ErrTitleRequired) {
		t.Fatalf("empty title: %v, want ErrTitleRequired", err)
	}
	if _, err := New("a", strings.Repeat("é", 201), "", nil, t0); !errors.Is(err, ErrTitleTooLong) {
		t.Fatalf("201 characters: %v, want ErrTitleTooLong", err)
	}
	if _, err := New("a", strings.Repeat("é", 200), "", nil, t0); err != nil {
		t.Fatalf("200 characters: %v", err)
	}
}

func TestDeadlineNeverInThePastWhenSet(t *testing.T) {
	if _, err := New("a", "x", "", ptr(t0.Add(-time.Second)), t0); !errors.Is(err, ErrDeadlineInPast) {
		t.Fatalf("create: %v, want ErrDeadlineInPast", err)
	}
	task, _ := New("a", "x", "", nil, t0)
	if err := task.Apply(Change{SetDeadline: true, Deadline: ptr(t0.Add(-time.Minute))}, t0); !errors.Is(err, ErrDeadlineInPast) {
		t.Fatalf("update: %v, want ErrDeadlineInPast", err)
	}
	if err := task.Apply(Change{SetDeadline: true, Deadline: ptr(t0.Add(time.Hour))}, t0); err != nil {
		t.Fatalf("future deadline: %v", err)
	}
}

func TestOverdue(t *testing.T) {
	task, _ := New("a", "x", "", ptr(t0.Add(time.Minute)), t0)
	if task.Overdue(t0) {
		t.Fatal("overdue before its deadline")
	}
	later := t0.Add(2 * time.Minute) // the deadline was a minute ago
	if !task.Overdue(later) {
		t.Fatal("open task past its deadline is not overdue")
	}
	if _, err := task.Complete("", later); err != nil {
		t.Fatal(err)
	}
	if task.Overdue(later) {
		t.Fatal("done task is overdue")
	}
}

func TestNoDeadlineNeverOverdue(t *testing.T) {
	task, _ := New("a", "x", "", nil, t0)
	if task.Overdue(t0.AddDate(10, 0, 0)) {
		t.Fatal("a task without a deadline is overdue")
	}
}

func TestCompleteAndReopen(t *testing.T) {
	task, _ := New("a", "x", "", nil, t0)
	if err := task.Reopen(t0); !errors.Is(err, ErrNotDone) {
		t.Fatalf("reopen open: %v, want ErrNotDone", err)
	}
	if _, err := task.Complete("r1", t0); err != nil {
		t.Fatal(err)
	}
	if task.State != Done || task.CompleteTime == nil {
		t.Fatalf("after complete: %+v", task)
	}
	if _, err := task.Complete("", t0); !errors.Is(err, ErrAlreadyDone) {
		t.Fatalf("complete done: %v, want ErrAlreadyDone", err)
	}
	if _, err := task.Complete("r2", t0); !errors.Is(err, ErrAlreadyDone) {
		t.Fatalf("complete done, other request: %v, want ErrAlreadyDone", err)
	}
	if replay, err := task.Complete("r1", t0); err != nil || !replay {
		t.Fatalf("retry of the completing request: replay=%v err=%v, want a replay", replay, err)
	}
	if err := task.Reopen(t0); err != nil {
		t.Fatal(err)
	}
	if task.State != Open || task.CompleteTime != nil {
		t.Fatalf("after reopen: %+v", task)
	}
}

func TestDoneTaskDeadlineImmutable(t *testing.T) {
	task, _ := New("a", "x", "", ptr(t0.Add(time.Hour)), t0)
	_, _ = task.Complete("", t0)
	for name, c := range map[string]Change{
		"move":  {SetDeadline: true, Deadline: ptr(t0.Add(2 * time.Hour))},
		"clear": {SetDeadline: true},
	} {
		if err := task.Apply(c, t0); !errors.Is(err, ErrDoneImmutable) {
			t.Errorf("%s: %v, want ErrDoneImmutable", name, err)
		}
	}
	title := "renamed"
	if err := task.Apply(Change{Title: &title}, t0); err != nil {
		t.Errorf("rename a done task: %v", err)
	}
}

func TestClearDeadline(t *testing.T) {
	task, _ := New("a", "x", "", ptr(t0.Add(time.Hour)), t0)
	if err := task.Apply(Change{SetDeadline: true}, t0); err != nil {
		t.Fatal(err)
	}
	if task.Deadline != nil {
		t.Fatal("deadline not cleared")
	}
}
