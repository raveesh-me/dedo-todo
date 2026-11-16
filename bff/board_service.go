package main

import (
	"context"
	"errors"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connectproto"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/protobuf/types/known/durationpb"

	webv1 "github.com/raveesh-me/dedo-todo/bff/gen/go/commerce/todo/web/v1"
	"github.com/raveesh-me/dedo-todo/bff/gen/go/commerce/todo/web/v1/webv1connect"
	tasksv1 "github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/tasks/v1"
	"github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/tasks/v1/tasksv1connect"
	time_trackingv1 "github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/time_tracking/v1"
	"github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/time_tracking/v1/time_trackingv1connect"
)

// errorDomain is the ErrorInfo domain of the BFF's own errors.
const errorDomain = "bff.todo.commerce.example.com"

// boardService implements BoardService: each method reads what its screen
// shows from the Todo API, as the caller, and never from a database. The
// API checks the caller's token on every call.
type boardService struct {
	webv1connect.UnimplementedBoardServiceHandler
	tasks   tasksv1connect.TaskServiceClient
	timers  time_trackingv1connect.TimerServiceClient
	entries time_trackingv1connect.TimeEntryServiceClient
}

// newBoardService returns the handler main.go registers. backend calls the
// Todo API as the caller.
func newBoardService(backend *connect.Client) *boardService {
	return &boardService{
		tasks:   tasksv1connect.NewTaskServiceClient(backend),
		timers:  time_trackingv1connect.NewTimerServiceClient(backend),
		entries: time_trackingv1connect.NewTimeEntryServiceClient(backend),
	}
}

var shows = map[webv1.LoadBoardRequest_Show]tasksv1.ListTasksRequest_Show{
	webv1.LoadBoardRequest_SHOW_OPEN:    tasksv1.ListTasksRequest_SHOW_OPEN,
	webv1.LoadBoardRequest_SHOW_DONE:    tasksv1.ListTasksRequest_SHOW_DONE,
	webv1.LoadBoardRequest_SHOW_OVERDUE: tasksv1.ListTasksRequest_SHOW_OVERDUE,
}

// LoadBoard implements BoardService.LoadBoard.
func (s *boardService) LoadBoard(ctx context.Context, req *webv1.LoadBoardRequest) (*webv1.LoadBoardResponse, error) {
	list, err := s.tasks.ListTasks(ctx, &tasksv1.ListTasksRequest{
		Limit: req.GetLimit(), Cursor: req.GetCursor(), Show: shows[req.GetShow()],
	})
	if err != nil {
		if reasonOf(err) == "INVALID_CURSOR" {
			return nil, bffError(connect.CodeInvalidArgument, "INVALID_CURSOR", "cursor is not a next_cursor this page returned")
		}
		return nil, passThrough(err)
	}
	resp := &webv1.LoadBoardResponse{Tasks: []*webv1.TaskCard{}, NextCursor: list.GetNextCursor()}
	titles := map[string]string{}
	for _, t := range list.GetTasks() {
		card := cardOf(t)
		// Tracked time needs the time tracking scopes: without them the
		// card shows none.
		if tracked, err := s.entries.CalculateTrackedTime(ctx, &time_trackingv1.CalculateTrackedTimeRequest{Task: t.GetName()}); err == nil {
			card.TrackedDuration = tracked.GetTotalDuration()
		} else if !optional(err) {
			return nil, passThrough(err)
		}
		titles[t.GetName()] = t.GetTitle()
		resp.Tasks = append(resp.Tasks, card)
	}
	timers, err := s.timers.ListTimers(ctx, &time_trackingv1.ListTimersRequest{})
	if err != nil && !optional(err) {
		return nil, passThrough(err)
	}
	for _, t := range timers.GetTimers() {
		resp.RunningTimer = &webv1.RunningTimer{
			Timer:     t.GetName(),
			Task:      t.GetTask(),
			TaskTitle: titles[t.GetTask()],
			StartTime: t.GetStartTime(),
		}
		if resp.RunningTimer.TaskTitle == "" {
			if got, err := s.tasks.GetTask(ctx, &tasksv1.GetTaskRequest{Name: t.GetTask()}); err == nil {
				resp.RunningTimer.TaskTitle = got.GetTask().GetTitle()
			}
		}
	}
	return resp, nil
}

// LoadTaskPage implements BoardService.LoadTaskPage.
func (s *boardService) LoadTaskPage(ctx context.Context, req *webv1.LoadTaskPageRequest) (*webv1.LoadTaskPageResponse, error) {
	got, err := s.tasks.GetTask(ctx, &tasksv1.GetTaskRequest{Name: req.GetTask()})
	if connect.CodeOf(err) == connect.CodeNotFound {
		return nil, bffError(connect.CodeNotFound, "TASK_NOT_FOUND", "no task named "+req.GetTask())
	}
	if err != nil {
		return nil, passThrough(err)
	}
	resp := &webv1.LoadTaskPageResponse{Task: cardOf(got.GetTask()), TimeEntries: []*webv1.EntryRow{}}
	tracked, err := s.entries.CalculateTrackedTime(ctx, &time_trackingv1.CalculateTrackedTimeRequest{Task: req.GetTask()})
	if err != nil && !optional(err) {
		return nil, passThrough(err)
	}
	resp.Task.TrackedDuration = tracked.GetTotalDuration()
	resp.EntryCount = tracked.GetEntryCount()
	entries, err := s.entries.ListTimeEntries(ctx, &time_trackingv1.ListTimeEntriesRequest{Task: req.GetTask(), Limit: 100})
	if err != nil && !optional(err) {
		return nil, passThrough(err)
	}
	for _, e := range entries.GetTimeEntries() {
		resp.TimeEntries = append(resp.TimeEntries, &webv1.EntryRow{
			TimeEntry: e.GetName(),
			StartTime: e.GetStartTime(),
			EndTime:   e.GetEndTime(),
			Duration:  e.GetDuration(),
			Manual:    e.GetOrigin() == time_trackingv1.TimeEntry_ORIGIN_MANUAL,
		})
	}
	return resp, nil
}

func cardOf(t *tasksv1.Task) *webv1.TaskCard {
	return &webv1.TaskCard{
		Task:            t.GetName(),
		Title:           t.GetTitle(),
		Notes:           t.GetNotes(),
		Done:            t.GetState() == tasksv1.Task_STATE_DONE,
		Overdue:         t.GetOverdue(),
		DeadlineTime:    t.GetDeadlineTime(),
		CompleteTime:    t.GetCompleteTime(),
		TrackedDuration: durationpb.New(0),
	}
}

// optional reports whether a failed read is one the screen does without:
// the caller may not read it.
func optional(err error) bool {
	return connect.CodeOf(err) == connect.CodePermissionDenied
}

// passThrough answers a failed backend call the frontend can only retry:
// BACKEND_UNAVAILABLE for the API being down, and the API's own refusal of
// the caller (no token, a denial) as it was.
func passThrough(err error) error {
	switch code := connect.CodeOf(err); code {
	case connect.CodeUnauthenticated, connect.CodePermissionDenied, connect.CodeInvalidArgument:
		var cerr *connect.Error
		if errors.As(err, &cerr) {
			return connect.NewError(code, cerr.Message())
		}
		return connect.NewError(code, code.String())
	}
	return bffError(connect.CodeUnavailable, "BACKEND_UNAVAILABLE", "the Todo API did not answer; try again")
}

func bffError(code connect.Code, reason, message string) error {
	err := connect.NewError(code, message)
	if d, derr := connectproto.NewErrorDetail(&errdetails.ErrorInfo{Reason: reason, Domain: errorDomain}); derr == nil {
		return err.WithDetail(d)
	}
	return err
}

func reasonOf(err error) string {
	var cerr *connect.Error
	if !errors.As(err, &cerr) {
		return ""
	}
	for _, d := range cerr.Details() {
		if m, err := connectproto.UnmarshalErrorDetail(d); err == nil {
			if info, ok := m.(*errdetails.ErrorInfo); ok {
				return info.GetReason()
			}
		}
	}
	return ""
}
