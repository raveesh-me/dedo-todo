package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	"github.com/GoHighLevel/commerce-auth/go/iamguard"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/raveesh-me/dedo-todo/app"
	tasksv1 "github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/tasks/v1"
	"github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/tasks/v1/tasksv1connect"
	ttv1 "github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/time_tracking/v1"
	"github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/time_tracking/v1/time_trackingv1connect"
	"github.com/raveesh-me/dedo-todo/internal/shared/apierr"
	"github.com/raveesh-me/dedo-todo/internal/shared/devauth"
)

// The integration tests run the server as main builds it, iamguard and
// dedo's interceptors included, on a Postgres database of their own. Set
// TEST_DATABASE_URL to a Postgres the tests may create databases in; make
// kind-up exposes the kind cluster's at 127.0.0.1:15432, and the Makefile
// sets it for make test.

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	admin := os.Getenv("TEST_DATABASE_URL")
	if admin == "" {
		fmt.Println("app: TEST_DATABASE_URL is not set; skipping the integration tests (make kind-up, then make test)")
		os.Exit(m.Run())
	}
	ctx := context.Background()
	name := fmt.Sprintf("todo_test_%d", time.Now().UnixNano())
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		fmt.Println("app: TEST_DATABASE_URL:", err)
		os.Exit(1)
	}
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		fmt.Println("app: create database:", err)
		os.Exit(1)
	}
	u, _ := url.Parse(admin)
	u.Path = "/" + name
	testPool, err = pgxpool.New(ctx, u.String())
	if err == nil {
		err = app.Migrate(ctx, testPool)
	}
	code := 1
	if err != nil {
		fmt.Println("app: migrate:", err)
	} else {
		code = m.Run()
	}
	if testPool != nil {
		testPool.Close()
	}
	_, _ = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	_ = conn.Close(ctx)
	os.Exit(code)
}

// clock is the server's clock, which a test moves.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

// env is one server, its clock and the locations its people act in, new
// for each test so that tests share no tasks or timers.
type env struct {
	t     *testing.T
	url   string
	clock *clock
	loc   string // alice, bob and reader's location
	other string // a second location only alice is in
}

func (e *env) alice() as      { return as{"alice", e.loc} }
func (e *env) aliceOther() as { return as{"alice", e.other} }
func (e *env) bob() as        { return as{"bob", e.loc} }
func (e *env) reader() as     { return as{"reader", e.loc} }

func newEnv(t *testing.T, authz iamguard.Authorizer) *env {
	t.Helper()
	if testPool == nil {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	loc, other := "loc-"+id, "loc-other-"+id
	if authz == nil {
		a, err := devauth.New(iamConfig(t), map[string]devauth.Person{
			"alice":  {User: "alice", Locations: []string{loc, other}, Scopes: []string{"*"}},
			"bob":    {User: "bob", Locations: []string{loc}, Scopes: []string{"*"}},
			"reader": {User: "reader", Locations: []string{loc}, Scopes: []string{"*.readonly"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		authz = a
	}
	c := &clock{t: time.Now().UTC().Truncate(time.Millisecond)}
	h, err := app.New(app.Config{Pool: testPool, Authorizer: authz, Now: c.Now})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &env{t: t, url: srv.URL, clock: c, loc: loc, other: other}
}

func iamConfig(t *testing.T) []byte {
	b, err := os.ReadFile("../gen/iam/iam-config.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// as is a caller: a development token and the location it acts in.
type as struct{ token, location string }


func (c as) headers(h interface{ Set(k, v string) }) {
	if c.token != "" {
		h.Set("Authorization", "Bearer "+c.token)
	}
	h.Set("location-id", c.location)
	h.Set("Version", app.APIVersion)
}

// clients are Connect clients of the three services, as one caller.
type clients struct {
	tasks   tasksv1connect.TaskServiceClient
	timers  time_trackingv1connect.TimerServiceClient
	entries time_trackingv1connect.TimeEntryServiceClient
}

func (e *env) connect(c as) clients {
	setHeaders := func(next connect.ClientFunc) connect.ClientFunc {
		return func(ctx context.Context, spec connect.Spec) (connect.ClientStream, error) {
			ctx, info := connect.NewClientContext(ctx)
			c.headers(info.RequestHeader())
			return next(ctx, spec)
		}
	}
	client := connect.NewClient(connecthttp.NewTransport(http.DefaultClient, e.url), setHeaders)
	return clients{
		tasks:   tasksv1connect.NewTaskServiceClient(client),
		timers:  time_trackingv1connect.NewTimerServiceClient(client),
		entries: time_trackingv1connect.NewTimeEntryServiceClient(client),
	}
}

func ctx() context.Context { return context.Background() }

// wantReason fails unless err has code and the ErrorInfo reason.
func wantReason(t *testing.T, err error, code connect.Code, reason string) {
	t.Helper()
	if err == nil {
		t.Fatalf("no error, want %s %s", code, reason)
	}
	if connect.CodeOf(err) != code || apierr.Reason(err) != reason {
		t.Fatalf("error = %v (%s, reason %q), want %s %s", err, connect.CodeOf(err), apierr.Reason(err), code, reason)
	}
}

func must[T any](t *testing.T) func(T, error) T {
	return func(v T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

func createTask(t *testing.T, c clients, title string, deadline *time.Time) *tasksv1.Task {
	t.Helper()
	task := &tasksv1.Task{Title: title}
	if deadline != nil {
		task.DeadlineTime = timestamppb.New(*deadline)
	}
	return must[*tasksv1.CreateTaskResponse](t)(c.tasks.CreateTask(ctx(), &tasksv1.CreateTaskRequest{Task: task})).GetTask()
}

func listed(t *testing.T, c clients, show tasksv1.ListTasksRequest_Show, name string) bool {
	t.Helper()
	resp := must[*tasksv1.ListTasksResponse](t)(c.tasks.ListTasks(ctx(), &tasksv1.ListTasksRequest{Show: show, Limit: 200}))
	for _, task := range resp.GetTasks() {
		if task.GetName() == name {
			return true
		}
	}
	return false
}

func runningTimers(t *testing.T, c clients) []*ttv1.Timer {
	t.Helper()
	return must[*ttv1.ListTimersResponse](t)(c.timers.ListTimers(ctx(), &ttv1.ListTimersRequest{})).GetTimers()
}

func entriesOf(t *testing.T, c clients, task string) []*ttv1.TimeEntry {
	t.Helper()
	return must[*ttv1.ListTimeEntriesResponse](t)(c.entries.ListTimeEntries(ctx(), &ttv1.ListTimeEntriesRequest{Task: task})).GetTimeEntries()
}

// Acceptance 1: a task with a deadline a minute in the past, still open,
// lists as overdue; completed, it does not.
func TestOverdue(t *testing.T) {
	e := newEnv(t, nil)
	c := e.connect(e.alice())
	deadline := e.clock.Now().Add(time.Minute)
	task := createTask(t, c, "Report", &deadline)
	if listed(t, c, tasksv1.ListTasksRequest_SHOW_OVERDUE, task.GetName()) {
		t.Fatal("overdue before its deadline")
	}
	e.clock.Add(2 * time.Minute) // the deadline is now a minute in the past
	if !listed(t, c, tasksv1.ListTasksRequest_SHOW_OVERDUE, task.GetName()) {
		t.Fatal("open task a minute past its deadline is not listed as overdue")
	}
	got := must[*tasksv1.GetTaskResponse](t)(c.tasks.GetTask(ctx(), &tasksv1.GetTaskRequest{Name: task.GetName()}))
	if !got.GetTask().GetOverdue() {
		t.Fatal("GetTask: overdue = false")
	}
	must[*tasksv1.CompleteTaskResponse](t)(c.tasks.CompleteTask(ctx(), &tasksv1.CompleteTaskRequest{Name: task.GetName()}))
	if listed(t, c, tasksv1.ListTasksRequest_SHOW_OVERDUE, task.GetName()) {
		t.Fatal("completed task is listed as overdue")
	}
	if !listed(t, c, tasksv1.ListTasksRequest_SHOW_DONE, task.GetName()) || listed(t, c, tasksv1.ListTasksRequest_SHOW_OPEN, task.GetName()) {
		t.Fatal("completed task is not listed as done only")
	}
}

// Acceptance 2: a task without a deadline is never overdue.
func TestNoDeadlineNeverOverdue(t *testing.T) {
	e := newEnv(t, nil)
	c := e.connect(e.alice())
	task := createTask(t, c, "Someday", nil)
	e.clock.Add(365 * 24 * time.Hour)
	if listed(t, c, tasksv1.ListTasksRequest_SHOW_OVERDUE, task.GetName()) {
		t.Fatal("task without a deadline listed as overdue")
	}
}

// Acceptance 3: a second timer is TIMER_ALREADY_RUNNING, and the first keeps
// running.
func TestOneTimer(t *testing.T) {
	e := newEnv(t, nil)
	c := e.connect(e.alice())
	a, b := createTask(t, c, "A", nil), createTask(t, c, "B", nil)
	first := must[*ttv1.StartTimerResponse](t)(c.timers.StartTimer(ctx(), &ttv1.StartTimerRequest{Task: a.GetName()})).GetTimer()
	_, err := c.timers.StartTimer(ctx(), &ttv1.StartTimerRequest{Task: b.GetName()})
	wantReason(t, err, connect.CodeFailedPrecondition, "TIMER_ALREADY_RUNNING")
	running := runningTimers(t, c)
	if len(running) != 1 || running[0].GetName() != first.GetName() || running[0].GetTask() != a.GetName() {
		t.Fatalf("running = %v, want only %s on %s", running, first.GetName(), a.GetName())
	}
	// Another person has a timer of their own.
	other := e.connect(e.bob())
	bt := createTask(t, other, "Bob's", nil)
	must[*ttv1.StartTimerResponse](t)(other.timers.StartTimer(ctx(), &ttv1.StartTimerRequest{Task: bt.GetName()}))
}

// Acceptance 4: completing or deleting a task whose timer is running stops
// the timer and writes exactly one time entry.
func TestCompletingOrDeletingStopsTheTimer(t *testing.T) {
	for _, how := range []string{"complete", "delete"} {
		t.Run(how, func(t *testing.T) {
			e := newEnv(t, nil)
			c := e.connect(e.alice())
			task := createTask(t, c, "Timed", nil)
			must[*ttv1.StartTimerResponse](t)(c.timers.StartTimer(ctx(), &ttv1.StartTimerRequest{Task: task.GetName()}))
			e.clock.Add(25 * time.Minute)
			switch how {
			case "complete":
				must[*tasksv1.CompleteTaskResponse](t)(c.tasks.CompleteTask(ctx(), &tasksv1.CompleteTaskRequest{Name: task.GetName()}))
			case "delete":
				must[*tasksv1.DeleteTaskResponse](t)(c.tasks.DeleteTask(ctx(), &tasksv1.DeleteTaskRequest{Name: task.GetName()}))
			}
			if running := runningTimers(t, c); len(running) != 0 {
				t.Fatalf("timer still running: %v", running)
			}
			entries := entriesOf(t, c, task.GetName())
			if len(entries) != 1 {
				t.Fatalf("%d time entries, want exactly 1", len(entries))
			}
			if got := entries[0]; got.GetOrigin() != ttv1.TimeEntry_ORIGIN_TIMER || got.GetDuration().AsDuration() != 25*time.Minute {
				t.Fatalf("entry = %v, want a 25m timer entry", got)
			}
		})
	}
}

// A timer on another task keeps running when a task is completed.
func TestCompletingLeavesOtherTimers(t *testing.T) {
	e := newEnv(t, nil)
	c := e.connect(e.alice())
	timed, other := createTask(t, c, "Timed", nil), createTask(t, c, "Other", nil)
	must[*ttv1.StartTimerResponse](t)(c.timers.StartTimer(ctx(), &ttv1.StartTimerRequest{Task: timed.GetName()}))
	must[*tasksv1.CompleteTaskResponse](t)(c.tasks.CompleteTask(ctx(), &tasksv1.CompleteTaskRequest{Name: other.GetName()}))
	if running := runningTimers(t, c); len(running) != 1 || running[0].GetTask() != timed.GetName() {
		t.Fatalf("running = %v, want the timer on %s", running, timed.GetName())
	}
	if n := len(entriesOf(t, c, other.GetName())); n != 0 {
		t.Fatalf("%d entries on the completed task, want 0", n)
	}
}

// A retried completion with the same request_id succeeds again and stops
// nothing twice.
func TestCompleteRetry(t *testing.T) {
	e := newEnv(t, nil)
	c := e.connect(e.alice())
	task := createTask(t, c, "Retry", nil)
	must[*ttv1.StartTimerResponse](t)(c.timers.StartTimer(ctx(), &ttv1.StartTimerRequest{Task: task.GetName()}))
	req := &tasksv1.CompleteTaskRequest{Name: task.GetName(), RequestId: "4f8b2c1e-9d3a-4e5f-8a7b-6c5d4e3f2a1b"}
	must[*tasksv1.CompleteTaskResponse](t)(c.tasks.CompleteTask(ctx(), req))
	must[*tasksv1.CompleteTaskResponse](t)(c.tasks.CompleteTask(ctx(), req))
	_, err := c.tasks.CompleteTask(ctx(), &tasksv1.CompleteTaskRequest{Name: task.GetName()})
	wantReason(t, err, connect.CodeFailedPrecondition, "TASK_ALREADY_DONE")
	if n := len(entriesOf(t, c, task.GetName())); n != 1 {
		t.Fatalf("%d entries, want 1", n)
	}
	del := &tasksv1.DeleteTaskRequest{Name: task.GetName(), RequestId: "9a8b7c6d-5e4f-4a3b-9c2d-1e0f9a8b7c6d"}
	must[*tasksv1.DeleteTaskResponse](t)(c.tasks.DeleteTask(ctx(), del))
	must[*tasksv1.DeleteTaskResponse](t)(c.tasks.DeleteTask(ctx(), del))
	_, err = c.tasks.DeleteTask(ctx(), &tasksv1.DeleteTaskRequest{Name: task.GetName()})
	wantReason(t, err, connect.CodeNotFound, "TASK_NOT_FOUND")
}

func TestTaskStateRules(t *testing.T) {
	e := newEnv(t, nil)
	c := e.connect(e.alice())
	task := createTask(t, c, "Rules", nil)
	_, err := c.tasks.ReopenTask(ctx(), &tasksv1.ReopenTaskRequest{Name: task.GetName()})
	wantReason(t, err, connect.CodeFailedPrecondition, "TASK_NOT_DONE")
	must[*tasksv1.CompleteTaskResponse](t)(c.tasks.CompleteTask(ctx(), &tasksv1.CompleteTaskRequest{Name: task.GetName()}))
	_, err = c.tasks.CompleteTask(ctx(), &tasksv1.CompleteTaskRequest{Name: task.GetName()})
	wantReason(t, err, connect.CodeFailedPrecondition, "TASK_ALREADY_DONE")

	// A done task's deadline cannot be set, moved or cleared.
	soon := timestamppb.New(e.clock.Now().Add(time.Hour))
	_, err = c.tasks.UpdateTask(ctx(), &tasksv1.UpdateTaskRequest{
		Task:       &tasksv1.Task{Name: task.GetName(), DeadlineTime: soon},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"deadline_time"}},
	})
	wantReason(t, err, connect.CodeFailedPrecondition, "TASK_DONE_IMMUTABLE")

	// A done task cannot be timed.
	_, err = c.timers.StartTimer(ctx(), &ttv1.StartTimerRequest{Task: task.GetName()})
	wantReason(t, err, connect.CodeFailedPrecondition, "TASK_NOT_OPEN")

	reopened := must[*tasksv1.ReopenTaskResponse](t)(c.tasks.ReopenTask(ctx(), &tasksv1.ReopenTaskRequest{Name: task.GetName()}))
	if reopened.GetTask().GetState() != tasksv1.Task_STATE_OPEN || reopened.GetTask().GetCompleteTime() != nil {
		t.Fatalf("reopened = %v", reopened.GetTask())
	}
	updated := must[*tasksv1.UpdateTaskResponse](t)(c.tasks.UpdateTask(ctx(), &tasksv1.UpdateTaskRequest{
		Task:       &tasksv1.Task{Name: task.GetName(), DeadlineTime: soon},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"deadline_time"}},
	}))
	if !updated.GetTask().GetDeadlineTime().AsTime().Equal(soon.AsTime()) || updated.GetTask().GetTitle() != "Rules" {
		t.Fatalf("updated = %v", updated.GetTask())
	}
	cleared := must[*tasksv1.UpdateTaskResponse](t)(c.tasks.UpdateTask(ctx(), &tasksv1.UpdateTaskRequest{
		Task:       &tasksv1.Task{Name: task.GetName()},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"deadline_time"}},
	}))
	if cleared.GetTask().GetDeadlineTime() != nil {
		t.Fatal("deadline not cleared")
	}
	_, err = c.tasks.UpdateTask(ctx(), &tasksv1.UpdateTaskRequest{
		Task:       &tasksv1.Task{Name: task.GetName()},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"state"}},
	})
	wantReason(t, err, connect.CodeInvalidArgument, "INVALID_UPDATE_MASK")
	_, err = c.tasks.UpdateTask(ctx(), &tasksv1.UpdateTaskRequest{
		Task:       &tasksv1.Task{Name: task.GetName()},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"title"}},
	})
	wantReason(t, err, connect.CodeInvalidArgument, "TITLE_REQUIRED")
}

func TestDeadlineInPast(t *testing.T) {
	e := newEnv(t, nil)
	c := e.connect(e.alice())
	past := e.clock.Now().Add(-time.Minute)
	_, err := c.tasks.CreateTask(ctx(), &tasksv1.CreateTaskRequest{Task: &tasksv1.Task{Title: "Late", DeadlineTime: timestamppb.New(past)}})
	wantReason(t, err, connect.CodeInvalidArgument, "DEADLINE_IN_PAST")
	task := createTask(t, c, "On time", nil)
	_, err = c.tasks.UpdateTask(ctx(), &tasksv1.UpdateTaskRequest{
		Task:       &tasksv1.Task{Name: task.GetName(), DeadlineTime: timestamppb.New(past)},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"deadline_time"}},
	})
	wantReason(t, err, connect.CodeInvalidArgument, "DEADLINE_IN_PAST")
	_, err = c.tasks.CreateTask(ctx(), &tasksv1.CreateTaskRequest{Task: &tasksv1.Task{}})
	wantReason(t, err, connect.CodeInvalidArgument, "TITLE_REQUIRED")
	_, err = c.tasks.CreateTask(ctx(), &tasksv1.CreateTaskRequest{Task: &tasksv1.Task{Title: "Dup"}, TaskId: "dup-task"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.tasks.CreateTask(ctx(), &tasksv1.CreateTaskRequest{Task: &tasksv1.Task{Title: "Dup"}, TaskId: "dup-task"})
	wantReason(t, err, connect.CodeAlreadyExists, "TASK_ALREADY_EXISTS")
}

func TestStartTimerOnMissingTask(t *testing.T) {
	e := newEnv(t, nil)
	c := e.connect(e.alice())
	_, err := c.timers.StartTimer(ctx(), &ttv1.StartTimerRequest{Task: "tasks/nope"})
	wantReason(t, err, connect.CodeNotFound, "TASK_NOT_FOUND")
	_, err = c.timers.StopTimer(ctx(), &ttv1.StopTimerRequest{Name: "timers/nope"})
	wantReason(t, err, connect.CodeNotFound, "TIMER_NOT_FOUND")
}

// Time entries are never edited: a correction is a delete plus a new manual
// entry, and a manual entry ends after it starts.
func TestTimeEntries(t *testing.T) {
	e := newEnv(t, nil)
	c := e.connect(e.alice())
	task := createTask(t, c, "Hours", nil)
	start := e.clock.Now().Add(-3 * time.Hour)
	entry := func(from, to time.Time) *ttv1.TimeEntry {
		return &ttv1.TimeEntry{Task: task.GetName(), StartTime: timestamppb.New(from), EndTime: timestamppb.New(to)}
	}
	_, err := c.entries.CreateTimeEntry(ctx(), &ttv1.CreateTimeEntryRequest{TimeEntry: entry(start, start)})
	wantReason(t, err, connect.CodeInvalidArgument, "INVALID_INTERVAL")
	_, err = c.entries.CreateTimeEntry(ctx(), &ttv1.CreateTimeEntryRequest{TimeEntry: entry(start, start.Add(-time.Minute))})
	wantReason(t, err, connect.CodeInvalidArgument, "INVALID_INTERVAL")
	_, err = c.entries.CreateTimeEntry(ctx(), &ttv1.CreateTimeEntryRequest{TimeEntry: &ttv1.TimeEntry{Task: "tasks/nope", StartTime: timestamppb.New(start), EndTime: timestamppb.New(start.Add(time.Hour))}})
	wantReason(t, err, connect.CodeNotFound, "TASK_NOT_FOUND")

	wrong := must[*ttv1.CreateTimeEntryResponse](t)(c.entries.CreateTimeEntry(ctx(), &ttv1.CreateTimeEntryRequest{TimeEntry: entry(start, start.Add(2*time.Hour))})).GetTimeEntry()
	must[*ttv1.CreateTimeEntryResponse](t)(c.entries.CreateTimeEntry(ctx(), &ttv1.CreateTimeEntryRequest{TimeEntry: entry(start, start.Add(30*time.Minute))}))
	tracked := must[*ttv1.CalculateTrackedTimeResponse](t)(c.entries.CalculateTrackedTime(ctx(), &ttv1.CalculateTrackedTimeRequest{Task: task.GetName()}))
	if tracked.GetTotalDuration().AsDuration() != 150*time.Minute || tracked.GetEntryCount() != 2 {
		t.Fatalf("tracked = %v, want 2h30m in 2 entries", tracked)
	}
	// Correct the two-hour entry to one hour: delete it, record a new one.
	must[*ttv1.DeleteTimeEntryResponse](t)(c.entries.DeleteTimeEntry(ctx(), &ttv1.DeleteTimeEntryRequest{Name: wrong.GetName()}))
	_, err = c.entries.GetTimeEntry(ctx(), &ttv1.GetTimeEntryRequest{Name: wrong.GetName()})
	wantReason(t, err, connect.CodeNotFound, "TIME_ENTRY_NOT_FOUND")
	must[*ttv1.CreateTimeEntryResponse](t)(c.entries.CreateTimeEntry(ctx(), &ttv1.CreateTimeEntryRequest{TimeEntry: entry(start, start.Add(time.Hour))}))
	tracked = must[*ttv1.CalculateTrackedTimeResponse](t)(c.entries.CalculateTrackedTime(ctx(), &ttv1.CalculateTrackedTimeRequest{Task: task.GetName()}))
	if tracked.GetTotalDuration().AsDuration() != 90*time.Minute || tracked.GetEntryCount() != 2 {
		t.Fatalf("tracked after correction = %v, want 1h30m in 2 entries", tracked)
	}
}

func TestPagination(t *testing.T) {
	e := newEnv(t, nil)
	c := e.connect(e.bob())
	var names []string
	for i := range 5 {
		names = append(names, createTask(t, c, fmt.Sprintf("Page %d", i), nil).GetName())
		e.clock.Add(time.Second)
	}
	var got []string
	cursor := ""
	for {
		resp := must[*tasksv1.ListTasksResponse](t)(c.tasks.ListTasks(ctx(), &tasksv1.ListTasksRequest{Limit: 2, Cursor: cursor}))
		for _, task := range resp.GetTasks() {
			got = append(got, task.GetName())
		}
		if cursor = resp.GetNextCursor(); cursor == "" {
			break
		}
	}
	seen := map[string]bool{}
	for _, n := range got {
		if seen[n] {
			t.Fatalf("%s listed twice", n)
		}
		seen[n] = true
	}
	for _, n := range names {
		if !seen[n] {
			t.Fatalf("%s never listed", n)
		}
	}
	_, err := c.tasks.ListTasks(ctx(), &tasksv1.ListTasksRequest{Cursor: "not-a-cursor"})
	wantReason(t, err, connect.CodeInvalidArgument, "INVALID_CURSOR")
}

// Tenants and people are apart: another person or location reads, lists,
// changes and deletes nothing of alice's, and gets NOT_FOUND for her names.
func TestIsolation(t *testing.T) {
	e := newEnv(t, nil)
	mine := e.connect(e.alice())
	task := createTask(t, mine, "Private", nil)
	for name, c := range map[string]clients{"another person": e.connect(e.bob()), "another location": e.connect(e.aliceOther())} {
		t.Run(name, func(t *testing.T) {
			_, err := c.tasks.GetTask(ctx(), &tasksv1.GetTaskRequest{Name: task.GetName()})
			wantReason(t, err, connect.CodeNotFound, "TASK_NOT_FOUND")
			if listed(t, c, tasksv1.ListTasksRequest_SHOW_UNSPECIFIED, task.GetName()) {
				t.Fatal("listed")
			}
			_, err = c.tasks.CompleteTask(ctx(), &tasksv1.CompleteTaskRequest{Name: task.GetName()})
			wantReason(t, err, connect.CodeNotFound, "TASK_NOT_FOUND")
			_, err = c.tasks.DeleteTask(ctx(), &tasksv1.DeleteTaskRequest{Name: task.GetName()})
			wantReason(t, err, connect.CodeNotFound, "TASK_NOT_FOUND")
			_, err = c.timers.StartTimer(ctx(), &ttv1.StartTimerRequest{Task: task.GetName()})
			wantReason(t, err, connect.CodeNotFound, "TASK_NOT_FOUND")
		})
	}
	got := must[*tasksv1.GetTaskResponse](t)(mine.tasks.GetTask(ctx(), &tasksv1.GetTaskRequest{Name: task.GetName()}))
	if got.GetTask().GetState() != tasksv1.Task_STATE_OPEN {
		t.Fatal("another tenant changed the task")
	}
}

// The guard, as main builds it: no token is Unauthenticated, a scope the
// caller lacks PermissionDenied, a location the caller is not in
// PermissionDenied, and IAM down Unavailable with IAM_UNAVAILABLE.
func TestAuth(t *testing.T) {
	e := newEnv(t, nil)
	_, err := e.connect(as{"", e.loc}).tasks.ListTasks(ctx(), &tasksv1.ListTasksRequest{})
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatalf("no token: %v, want Unauthenticated", err)
	}
	_, err = e.connect(e.reader()).tasks.CreateTask(ctx(), &tasksv1.CreateTaskRequest{Task: &tasksv1.Task{Title: "x"}})
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("reader writes: %v, want PermissionDenied", err)
	}
	if _, err := e.connect(e.reader()).tasks.ListTasks(ctx(), &tasksv1.ListTasksRequest{}); err != nil {
		t.Fatalf("reader reads: %v", err)
	}
	_, err = e.connect(as{"bob", e.other}).tasks.ListTasks(ctx(), &tasksv1.ListTasksRequest{})
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("bob in a location he is not in: %v, want PermissionDenied", err)
	}

	down := newEnv(t, unavailable{})
	_, err = down.connect(down.alice()).tasks.ListTasks(ctx(), &tasksv1.ListTasksRequest{})
	wantReason(t, err, connect.CodeUnavailable, iamguard.ReasonIAMUnavailable)
}

type unavailable struct{}

func (unavailable) Authorize(context.Context, iamguard.Request) (iamguard.Decision, error) {
	return iamguard.Decision{}, iamguard.ErrUnavailable
}

// Acceptance 6: the same create, track and complete flow works over
// Connect, REST and MCP, with the same errors.
func TestSameBehaviourEverywhere(t *testing.T) {
	for _, transport := range []string{"connect", "rest", "mcp"} {
		t.Run(transport, func(t *testing.T) {
			e := newEnv(t, nil)
			caller := e.bob()
			var call func(procedure string, in map[string]any) (map[string]any, string)
			switch transport {
			case "connect":
				call = e.connectJSON(caller)
			case "rest":
				call = e.rest(caller)
			case "mcp":
				call = e.mcp(caller)
			}
			// Leave no timer of bob's running from another subtest.
			if list, _ := call("ListTimers", map[string]any{}); list != nil {
				for _, tm := range asList(list["timers"]) {
					call("StopTimer", map[string]any{"name": tm["name"]})
				}
			}
			task, reason := call("CreateTask", map[string]any{"task": map[string]any{"title": "Flow over " + transport}})
			if reason != "" {
				t.Fatalf("create: %s", reason)
			}
			name := task["name"].(string)
			if _, reason := call("StartTimer", map[string]any{"task": name}); reason != "" {
				t.Fatalf("start: %s", reason)
			}
			if _, reason := call("StartTimer", map[string]any{"task": name}); reason != "TIMER_ALREADY_RUNNING" {
				t.Fatalf("second start: reason %q, want TIMER_ALREADY_RUNNING", reason)
			}
			e.clock.Add(10 * time.Minute)
			done, reason := call("CompleteTask", map[string]any{"name": name})
			if reason != "" || done["state"] != "STATE_DONE" {
				t.Fatalf("complete: %v %s", done, reason)
			}
			if _, reason := call("CompleteTask", map[string]any{"name": name}); reason != "TASK_ALREADY_DONE" {
				t.Fatalf("complete again: reason %q, want TASK_ALREADY_DONE", reason)
			}
			if _, reason := call("StartTimer", map[string]any{"task": name}); reason != "TASK_NOT_OPEN" {
				t.Fatalf("start on done: reason %q, want TASK_NOT_OPEN", reason)
			}
			list, reason := call("ListTimeEntries", map[string]any{"task": name})
			if reason != "" || len(asList(list["time_entries"])) != 1 {
				t.Fatalf("entries: %v %s, want exactly 1", list, reason)
			}
		})
	}
}

func asList(v any) []map[string]any {
	items, _ := v.([]any)
	var out []map[string]any
	for _, i := range items {
		if m, ok := i.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

var services = map[string]string{
	"CreateTask": "commerce.todo.tasks.v1.TaskService", "CompleteTask": "commerce.todo.tasks.v1.TaskService",
	"StartTimer": "commerce.todo.time_tracking.v1.TimerService", "StopTimer": "commerce.todo.time_tracking.v1.TimerService",
	"ListTimers": "commerce.todo.time_tracking.v1.TimerService", "ListTimeEntries": "commerce.todo.time_tracking.v1.TimeEntryService",
}

// unwrap is the resource a REST response or MCP result carries for the
// Connect response's single resource field.
var unwrap = map[string]string{"CreateTask": "task", "CompleteTask": "task", "StartTimer": "timer", "StopTimer": "time_entry"}

// connectJSON calls a procedure over the Connect protocol with JSON, and
// returns the response with proto field names.
func (e *env) connectJSON(c as) func(string, map[string]any) (map[string]any, string) {
	return func(procedure string, in map[string]any) (map[string]any, string) {
		out, reason := e.post(c, "/"+services[procedure]+"/"+procedure, "application/json", in)
		if k := unwrap[procedure]; k != "" && out != nil {
			out, _ = out[camel(k)].(map[string]any)
		}
		return snake(out), reason
	}
}

// rest calls a procedure over its google.api.http binding.
func (e *env) rest(c as) func(string, map[string]any) (map[string]any, string) {
	return func(procedure string, in map[string]any) (map[string]any, string) {
		var method, path string
		var body any
		switch procedure {
		case "CreateTask":
			method, path, body = "POST", "/commerce/todo/v1/tasks", in["task"]
		case "CompleteTask":
			method, path, body = "POST", "/commerce/todo/v1/"+in["name"].(string)+":complete", map[string]any{}
		case "StartTimer":
			method, path, body = "POST", "/commerce/todo/v1/timers:startTimer", in
		case "StopTimer":
			method, path, body = "POST", "/commerce/todo/v1/"+in["name"].(string)+":stop", map[string]any{}
		case "ListTimers":
			method, path = "GET", "/commerce/todo/v1/timers"
		case "ListTimeEntries":
			method, path = "GET", "/commerce/todo/v1/time-entries?task="+url.QueryEscape(in["task"].(string))
		}
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, _ := http.NewRequest(method, e.url+path, r)
		c.headers(req.Header)
		req.Header.Set("Content-Type", "application/json")
		out, reason := e.do(req)
		return snake(out), reason
	}
}

// mcp calls a procedure as its MCP tool.
func (e *env) mcp(c as) func(string, map[string]any) (map[string]any, string) {
	return func(procedure string, in map[string]any) (map[string]any, string) {
		rpc := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
			"params": map[string]any{"name": snakeName(procedure), "arguments": in}}
		b, _ := json.Marshal(rpc)
		req, _ := http.NewRequest("POST", e.url+"/mcp", bytes.NewReader(b))
		c.headers(req.Header)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			e.t.Fatal(err)
		}
		defer resp.Body.Close()
		var env struct {
			Result struct {
				IsError           bool             `json:"isError"`
				StructuredContent map[string]any   `json:"structuredContent"`
				Content           []map[string]any `json:"content"`
			} `json:"result"`
			Error any `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil || env.Error != nil {
			e.t.Fatalf("mcp %s: %v %v", procedure, err, env.Error)
		}
		out := env.Result.StructuredContent
		if out == nil && len(env.Result.Content) > 0 {
			_ = json.Unmarshal([]byte(fmt.Sprint(env.Result.Content[0]["text"])), &out)
		}
		if env.Result.IsError {
			if out == nil {
				_ = json.Unmarshal([]byte(fmt.Sprint(env.Result.Content[0]["text"])), &out)
			}
			r, _ := out["reason"].(string)
			if r == "" {
				r = fmt.Sprint(out["code"])
			}
			return nil, r
		}
		if k := unwrap[procedure]; k != "" {
			out, _ = out[k].(map[string]any)
		}
		return snake(out), ""
	}
}

func (e *env) post(c as, path, contentType string, in any) (map[string]any, string) {
	b, _ := json.Marshal(in)
	req, _ := http.NewRequest("POST", e.url+path, bytes.NewReader(b))
	c.headers(req.Header)
	req.Header.Set("Content-Type", contentType)
	return e.do(req)
}

// do sends req and returns its JSON body, or the reason of the error it
// carries (google.rpc.Status over REST, Connect's error JSON over Connect).
func (e *env) do(req *http.Request) (map[string]any, string) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode/100 == 2 {
		return out, ""
	}
	for _, d := range asList(out["details"]) {
		if r, ok := d["reason"].(string); ok {
			return nil, r
		}
		// Connect's JSON carries the detail as base64 protobuf, with a debug
		// rendering.
		if dbg, ok := d["debug"].(map[string]any); ok {
			if r, ok := dbg["reason"].(string); ok {
				return nil, r
			}
		}
	}
	return nil, fmt.Sprintf("HTTP %d %v", resp.StatusCode, out)
}

// snake rewrites a JSON object's lowerCamelCase keys, as Connect and REST
// write them, to the proto field names MCP results use.
func snake(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := map[string]any{}
	for k, v := range m {
		switch x := v.(type) {
		case map[string]any:
			v = snake(x)
		case []any:
			var items []any
			for _, i := range x {
				if im, ok := i.(map[string]any); ok {
					items = append(items, snake(im))
				} else {
					items = append(items, i)
				}
			}
			v = items
		}
		out[snakeName(k)] = v
	}
	return out
}

func snakeName(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func camel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
}

// Storage scopes every call: after a call, no pooled connection carries a
// tenant or a module's role; under a module's role, row-level security
// refuses a row for another tenant; and one module's role cannot read the
// other module's schema.
func TestStorageScoping(t *testing.T) {
	e := newEnv(t, nil)
	c := e.connect(e.alice())
	createTask(t, c, "Scoped", nil)

	for _, conn := range testPool.AcquireAllIdle(ctx()) {
		var loc, actor *string
		var user string
		err := conn.QueryRow(ctx(), "SELECT current_setting('app.location_id', true), current_setting('app.actor', true), current_user").Scan(&loc, &actor, &user)
		conn.Release()
		if err != nil {
			t.Fatal(err)
		}
		if (loc != nil && *loc != "") || (actor != nil && *actor != "") || user != "postgres" {
			t.Fatalf("pooled connection carries location %v, actor %v, role %s", deref(loc), deref(actor), user)
		}
	}

	scoped := func(role string, fn func(pgx.Tx) error) error {
		return pgx.BeginFunc(ctx(), testPool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx(), "SELECT set_config('app.location_id', $1, true), set_config('app.actor', 'User:alice', true), set_config('role', $2, true)", e.loc, role); err != nil {
				return err
			}
			return fn(tx)
		})
	}
	err := scoped("tasks_app", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx(), `INSERT INTO tasks.tasks (location_id, actor, id, title, state, create_time, update_time)
			VALUES ($1, 'User:alice', 'smuggled', 'x', 'OPEN', now(), now())`, e.other)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "row-level security") {
		t.Fatalf("insert for another location: %v, want a row-level security violation", err)
	}
	var n int
	if err := scoped("tasks_app", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx(), "SELECT count(*) FROM tasks.tasks").Scan(&n)
	}); err != nil || n != 1 {
		t.Fatalf("tasks_app sees %d rows (%v), want only alice's 1 in this location", n, err)
	}
	err = scoped("time_tracking_app", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx(), "SELECT count(*) FROM tasks.tasks").Scan(&n)
	})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("time_tracking_app reads the tasks schema: %v, want permission denied", err)
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
