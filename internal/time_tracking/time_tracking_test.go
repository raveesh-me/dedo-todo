package time_tracking

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)

func TestStopWritesOneEntry(t *testing.T) {
	timer := Timer{ID: "t", Task: "tasks/a", StartTime: t0}
	e := timer.Stop("e", t0.Add(90*time.Minute))
	if e.Task != "tasks/a" || e.Origin != FromTimer || e.Duration() != 90*time.Minute {
		t.Fatalf("entry = %+v", e)
	}
}

func TestStopNeverNegative(t *testing.T) {
	e := Timer{StartTime: t0}.Stop("e", t0.Add(-time.Second))
	if e.Duration() != 0 {
		t.Fatalf("duration = %s, want 0", e.Duration())
	}
}

func TestManualEntryInterval(t *testing.T) {
	if _, err := NewManualEntry("e", "tasks/a", t0, t0, t0); !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("end == start: %v, want ErrInvalidInterval", err)
	}
	if _, err := NewManualEntry("e", "tasks/a", t0, t0.Add(-time.Minute), t0); !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("end before start: %v, want ErrInvalidInterval", err)
	}
	e, err := NewManualEntry("e", "tasks/a", t0, t0.Add(time.Hour), t0)
	if err != nil || e.Origin != Manual || e.Duration() != time.Hour {
		t.Fatalf("entry = %+v, err = %v", e, err)
	}
}
