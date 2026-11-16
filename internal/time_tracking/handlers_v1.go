package time_tracking

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	time_trackingv1 "github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/time_tracking/v1"
	"github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/time_tracking/v1/time_trackingv1connect"
	"github.com/raveesh-me/dedo-todo/internal/shared/apierr"
	"github.com/raveesh-me/dedo-todo/internal/shared/db"
	"github.com/raveesh-me/dedo-todo/internal/shared/ids"
	"github.com/raveesh-me/dedo-todo/internal/shared/page"
)

const (
	timerPrefix = "timers/"
	entryPrefix = "time-entries/"
)

// module holds what both of the module's services share.
type module struct {
	pool  *pgxpool.Pool
	tasks Tasks
	now   func() time.Time
	st    store
}

func newModule(pool *pgxpool.Pool, tasks Tasks, now func() time.Time) module {
	if now == nil {
		now = time.Now
	}
	return module{pool: pool, tasks: tasks, now: func() time.Time { return now().UTC() }}
}

func (m module) tx(ctx context.Context, fn func(pgx.Tx, db.Tenant) error) error {
	return db.InTenantTx(ctx, m.pool, role, fn)
}

// TimerService implements commerce.todo.time_tracking.v1.TimerService.
type TimerService struct {
	time_trackingv1connect.UnimplementedTimerServiceHandler
	module
}

// NewTimerService returns the handler the composition root registers.
// tasks answers whether a task is open.
func NewTimerService(pool *pgxpool.Pool, tasks Tasks, now func() time.Time) *TimerService {
	return &TimerService{module: newModule(pool, tasks, now)}
}

// StartTimer implements TimerService.StartTimer.
func (s *TimerService) StartTimer(ctx context.Context, req *time_trackingv1.StartTimerRequest) (*time_trackingv1.StartTimerResponse, error) {
	open, err := s.tasks.Open(ctx, req.GetTask())
	switch {
	case errors.Is(err, ErrTaskNotFound):
		return nil, apierr.New(connect.CodeNotFound, "TASK_NOT_FOUND", "no task named "+req.GetTask())
	case err != nil:
		return nil, apierr.New(connect.CodeUnavailable, "TASKS_UNAVAILABLE", "could not read the task; try again")
	case !open:
		return nil, apierr.New(connect.CodeFailedPrecondition, "TASK_NOT_OPEN", "the task is done: only an open task can be timed")
	}
	now := s.now()
	t := Timer{ID: ids.New(now), Task: req.GetTask(), StartTime: now}
	err = s.tx(ctx, func(tx pgx.Tx, ten db.Tenant) error { return s.st.startTimer(ctx, tx, ten, t) })
	if errors.Is(err, errAlreadyRunning) {
		return nil, apierr.New(connect.CodeFailedPrecondition, "TIMER_ALREADY_RUNNING", "a timer is already running: stop it before starting another")
	}
	if err != nil {
		return nil, err
	}
	return &time_trackingv1.StartTimerResponse{Timer: timerProto(t)}, nil
}

// GetTimer implements TimerService.GetTimer.
func (s *TimerService) GetTimer(ctx context.Context, req *time_trackingv1.GetTimerRequest) (*time_trackingv1.GetTimerResponse, error) {
	id, ok := idOf(req.GetName(), timerPrefix)
	if !ok {
		return nil, timerNotFound(req.GetName())
	}
	var t Timer
	err := s.tx(ctx, func(tx pgx.Tx, _ db.Tenant) (err error) {
		t, err = s.st.getTimer(ctx, tx, id)
		return err
	})
	if errors.Is(err, errNotFound) {
		return nil, timerNotFound(req.GetName())
	}
	if err != nil {
		return nil, err
	}
	return &time_trackingv1.GetTimerResponse{Timer: timerProto(t)}, nil
}

// ListTimers implements TimerService.ListTimers: the running timer, if any.
func (s *TimerService) ListTimers(ctx context.Context, req *time_trackingv1.ListTimersRequest) (*time_trackingv1.ListTimersResponse, error) {
	// A person has at most one timer, so every list fits on one page: the
	// only cursor this API returns is empty.
	if req.GetCursor() != "" {
		return nil, apierr.New(connect.CodeInvalidArgument, "INVALID_CURSOR", "cursor is not a next_cursor this API returned")
	}
	resp := &time_trackingv1.ListTimersResponse{Timers: []*time_trackingv1.Timer{}}
	err := s.tx(ctx, func(tx pgx.Tx, _ db.Tenant) error {
		t, err := s.st.runningTimer(ctx, tx)
		if err == nil {
			resp.Timers = append(resp.Timers, timerProto(t))
		}
		if errors.Is(err, errNotFound) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// StopTimer implements TimerService.StopTimer: the timer goes, and its one
// time entry is written, in one transaction.
func (s *TimerService) StopTimer(ctx context.Context, req *time_trackingv1.StopTimerRequest) (*time_trackingv1.StopTimerResponse, error) {
	id, ok := idOf(req.GetName(), timerPrefix)
	if !ok {
		return nil, timerNotFound(req.GetName())
	}
	now := s.now()
	var e TimeEntry
	err := s.tx(ctx, func(tx pgx.Tx, ten db.Tenant) error {
		t, err := s.st.takeTimer(ctx, tx, id)
		if err != nil {
			return err
		}
		e = t.Stop(ids.New(now), now)
		return s.st.insertEntry(ctx, tx, ten, e)
	})
	if errors.Is(err, errNotFound) {
		return nil, timerNotFound(req.GetName())
	}
	if err != nil {
		return nil, err
	}
	return &time_trackingv1.StopTimerResponse{TimeEntry: entryProto(e)}, nil
}

func timerNotFound(name string) error {
	return apierr.New(connect.CodeNotFound, "TIMER_NOT_FOUND", "no running timer named "+name)
}

// TimeEntryService implements commerce.todo.time_tracking.v1.TimeEntryService.
type TimeEntryService struct {
	time_trackingv1connect.UnimplementedTimeEntryServiceHandler
	module
}

// NewTimeEntryService returns the handler the composition root registers.
// tasks answers whether a task exists.
func NewTimeEntryService(pool *pgxpool.Pool, tasks Tasks, now func() time.Time) *TimeEntryService {
	return &TimeEntryService{module: newModule(pool, tasks, now)}
}

// CreateTimeEntry implements TimeEntryService.CreateTimeEntry.
func (s *TimeEntryService) CreateTimeEntry(ctx context.Context, req *time_trackingv1.CreateTimeEntryRequest) (*time_trackingv1.CreateTimeEntryResponse, error) {
	in := req.GetTimeEntry()
	now := s.now()
	id := req.GetTimeEntryId()
	if id == "" {
		id = ids.New(now)
	}
	e, err := NewManualEntry(id, in.GetTask(), in.GetStartTime().AsTime(), in.GetEndTime().AsTime(), now)
	if err != nil || in.GetStartTime() == nil || in.GetEndTime() == nil {
		return nil, apierr.New(connect.CodeInvalidArgument, "INVALID_INTERVAL", "start_time and end_time are required, and end_time must be after start_time")
	}
	if _, err := s.tasks.Open(ctx, in.GetTask()); errors.Is(err, ErrTaskNotFound) {
		return nil, apierr.New(connect.CodeNotFound, "TASK_NOT_FOUND", "no task named "+in.GetTask())
	} else if err != nil {
		return nil, apierr.New(connect.CodeUnavailable, "TASKS_UNAVAILABLE", "could not read the task; try again")
	}
	err = s.tx(ctx, func(tx pgx.Tx, ten db.Tenant) error { return s.st.insertEntry(ctx, tx, ten, e) })
	if errors.Is(err, errExists) {
		return nil, apierr.New(connect.CodeAlreadyExists, "TIME_ENTRY_ALREADY_EXISTS", "a time entry named "+entryPrefix+id+" already exists")
	}
	if err != nil {
		return nil, err
	}
	return &time_trackingv1.CreateTimeEntryResponse{TimeEntry: entryProto(e)}, nil
}

// GetTimeEntry implements TimeEntryService.GetTimeEntry.
func (s *TimeEntryService) GetTimeEntry(ctx context.Context, req *time_trackingv1.GetTimeEntryRequest) (*time_trackingv1.GetTimeEntryResponse, error) {
	id, ok := idOf(req.GetName(), entryPrefix)
	if !ok {
		return nil, entryNotFound(req.GetName())
	}
	var e TimeEntry
	err := s.tx(ctx, func(tx pgx.Tx, _ db.Tenant) (err error) {
		e, err = s.st.getEntry(ctx, tx, id)
		return err
	})
	if errors.Is(err, errNotFound) {
		return nil, entryNotFound(req.GetName())
	}
	if err != nil {
		return nil, err
	}
	return &time_trackingv1.GetTimeEntryResponse{TimeEntry: entryProto(e)}, nil
}

// ListTimeEntries implements TimeEntryService.ListTimeEntries.
func (s *TimeEntryService) ListTimeEntries(ctx context.Context, req *time_trackingv1.ListTimeEntriesRequest) (*time_trackingv1.ListTimeEntriesResponse, error) {
	after, more, err := page.Decode(req.GetCursor())
	if err != nil {
		return nil, apierr.New(connect.CodeInvalidArgument, "INVALID_CURSOR", "cursor is not a next_cursor this API returned")
	}
	var from *page.Key
	if more {
		from = &after
	}
	limit := page.Limit(req.GetLimit())
	var list []TimeEntry
	err = s.tx(ctx, func(tx pgx.Tx, _ db.Tenant) (err error) {
		list, err = s.st.listEntries(ctx, tx, req.GetTask(), from, limit)
		return err
	})
	if err != nil {
		return nil, err
	}
	resp := &time_trackingv1.ListTimeEntriesResponse{TimeEntries: []*time_trackingv1.TimeEntry{}}
	if len(list) > limit {
		list = list[:limit]
		last := list[limit-1]
		resp.NextCursor = page.Encode(page.Key{Time: last.StartTime, ID: last.ID})
	}
	for _, e := range list {
		resp.TimeEntries = append(resp.TimeEntries, entryProto(e))
	}
	return resp, nil
}

// DeleteTimeEntry implements TimeEntryService.DeleteTimeEntry.
func (s *TimeEntryService) DeleteTimeEntry(ctx context.Context, req *time_trackingv1.DeleteTimeEntryRequest) (*time_trackingv1.DeleteTimeEntryResponse, error) {
	id, ok := idOf(req.GetName(), entryPrefix)
	if !ok {
		return nil, entryNotFound(req.GetName())
	}
	err := s.tx(ctx, func(tx pgx.Tx, _ db.Tenant) error { return s.st.deleteEntry(ctx, tx, id) })
	if errors.Is(err, errNotFound) {
		return nil, entryNotFound(req.GetName())
	}
	if err != nil {
		return nil, err
	}
	return &time_trackingv1.DeleteTimeEntryResponse{}, nil
}

// CalculateTrackedTime implements TimeEntryService.CalculateTrackedTime.
func (s *TimeEntryService) CalculateTrackedTime(ctx context.Context, req *time_trackingv1.CalculateTrackedTimeRequest) (*time_trackingv1.CalculateTrackedTimeResponse, error) {
	if _, err := s.tasks.Open(ctx, req.GetTask()); errors.Is(err, ErrTaskNotFound) {
		return nil, apierr.New(connect.CodeNotFound, "TASK_NOT_FOUND", "no task named "+req.GetTask())
	} else if err != nil {
		return nil, apierr.New(connect.CodeUnavailable, "TASKS_UNAVAILABLE", "could not read the task; try again")
	}
	var total time.Duration
	var n int
	err := s.tx(ctx, func(tx pgx.Tx, _ db.Tenant) (err error) {
		total, n, err = s.st.trackedTime(ctx, tx, req.GetTask())
		return err
	})
	if err != nil {
		return nil, err
	}
	return &time_trackingv1.CalculateTrackedTimeResponse{
		Task:          req.GetTask(),
		TotalDuration: durationpb.New(total),
		EntryCount:    int32(n),
	}, nil
}

func entryNotFound(name string) error {
	return apierr.New(connect.CodeNotFound, "TIME_ENTRY_NOT_FOUND", "no time entry named "+name)
}

func idOf(name, prefix string) (string, bool) {
	id, ok := strings.CutPrefix(name, prefix)
	return id, ok && id != "" && !strings.Contains(id, "/")
}

func timerProto(t Timer) *time_trackingv1.Timer {
	return &time_trackingv1.Timer{Name: timerPrefix + t.ID, Task: t.Task, StartTime: timestamppb.New(t.StartTime)}
}

func entryProto(e TimeEntry) *time_trackingv1.TimeEntry {
	origin := time_trackingv1.TimeEntry_ORIGIN_TIMER
	if e.Origin == Manual {
		origin = time_trackingv1.TimeEntry_ORIGIN_MANUAL
	}
	return &time_trackingv1.TimeEntry{
		Name:       entryPrefix + e.ID,
		Task:       e.Task,
		StartTime:  timestamppb.New(e.StartTime),
		EndTime:    timestamppb.New(e.EndTime),
		Duration:   durationpb.New(e.Duration()),
		Origin:     origin,
		CreateTime: timestamppb.New(e.CreateTime),
	}
}
