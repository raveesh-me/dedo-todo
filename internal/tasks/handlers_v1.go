package tasks

import (
	"context"
	"errors"
	"strings"
	"time"

	"connectrpc.com/connect/v2"
	"github.com/GoHighLevel/dedo/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/timestamppb"

	tasksv1 "github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/tasks/v1"
	"github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/tasks/v1/tasksv1connect"
	"github.com/raveesh-me/dedo-todo/internal/shared/apierr"
	"github.com/raveesh-me/dedo-todo/internal/shared/db"
	"github.com/raveesh-me/dedo-todo/internal/shared/ids"
	"github.com/raveesh-me/dedo-todo/internal/shared/page"
)

// The event types TaskService's commands announce, as their RPCs declare
// them in emits.
const (
	EventCompleted = "commerce.todo.tasks.task.completed"
	EventDeleted   = "commerce.todo.tasks.task.deleted"
)

const namePrefix = "tasks/"

// TaskService implements commerce.todo.tasks.v1.TaskService over the Task
// aggregate. Scopes, validation and auth are the interceptors'; it keeps
// the task's rules and its storage.
type TaskService struct {
	tasksv1connect.UnimplementedTaskServiceHandler
	pool   *pgxpool.Pool
	events *events.Publisher
	now    func() time.Time
	st     store
}

// NewTaskService returns the handler the composition root registers. now
// is the clock deadlines and overdue are judged by.
func NewTaskService(pool *pgxpool.Pool, pub *events.Publisher, now func() time.Time) *TaskService {
	if now == nil {
		now = time.Now
	}
	return &TaskService{pool: pool, events: pub, now: func() time.Time { return now().UTC() }}
}

func (s *TaskService) tx(ctx context.Context, fn func(pgx.Tx, db.Tenant) error) error {
	return db.InTenantTx(ctx, s.pool, role, fn)
}

// CreateTask implements TaskService.CreateTask.
func (s *TaskService) CreateTask(ctx context.Context, req *tasksv1.CreateTaskRequest) (*tasksv1.CreateTaskResponse, error) {
	now := s.now()
	id := req.GetTaskId()
	if id == "" {
		id = ids.New(now)
	}
	in := req.GetTask()
	t, err := New(id, in.GetTitle(), in.GetNotes(), timePtr(in.GetDeadlineTime()), now)
	switch {
	case errors.Is(err, ErrTitleRequired):
		return nil, apierr.New(connect.CodeInvalidArgument, "TITLE_REQUIRED", "a task needs a title")
	case errors.Is(err, ErrTitleTooLong):
		return nil, apierr.New(connect.CodeInvalidArgument, "TITLE_REQUIRED", "a title has 1 to 200 characters")
	case errors.Is(err, ErrDeadlineInPast):
		return nil, apierr.New(connect.CodeInvalidArgument, "DEADLINE_IN_PAST", "the deadline is in the past: set one that is still ahead")
	case err != nil:
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx, ten db.Tenant) error { return s.st.insert(ctx, tx, ten, t) })
	if errors.Is(err, errExists) {
		return nil, apierr.New(connect.CodeAlreadyExists, "TASK_ALREADY_EXISTS", "a task named "+namePrefix+id+" already exists")
	}
	if err != nil {
		return nil, err
	}
	return &tasksv1.CreateTaskResponse{Task: toProto(t, now)}, nil
}

// GetTask implements TaskService.GetTask.
func (s *TaskService) GetTask(ctx context.Context, req *tasksv1.GetTaskRequest) (*tasksv1.GetTaskResponse, error) {
	id, ok := idOf(req.GetName())
	if !ok {
		return nil, taskNotFound(req.GetName())
	}
	var t Task
	err := s.tx(ctx, func(tx pgx.Tx, _ db.Tenant) (err error) {
		t, err = s.st.get(ctx, tx, id, false)
		return err
	})
	if errors.Is(err, errNotFound) {
		return nil, taskNotFound(req.GetName())
	}
	if err != nil {
		return nil, err
	}
	return &tasksv1.GetTaskResponse{Task: toProto(t, s.now())}, nil
}

// ListTasks implements TaskService.ListTasks.
func (s *TaskService) ListTasks(ctx context.Context, req *tasksv1.ListTasksRequest) (*tasksv1.ListTasksResponse, error) {
	after, more, err := page.Decode(req.GetCursor())
	if err != nil {
		return nil, apierr.New(connect.CodeInvalidArgument, "INVALID_CURSOR", "cursor is not a next_cursor this API returned")
	}
	var from *page.Key
	if more {
		from = &after
	}
	which := map[tasksv1.ListTasksRequest_Show]show{
		tasksv1.ListTasksRequest_SHOW_OPEN:    showOpen,
		tasksv1.ListTasksRequest_SHOW_DONE:    showDone,
		tasksv1.ListTasksRequest_SHOW_OVERDUE: showOverdue,
	}[req.GetShow()]
	limit := page.Limit(req.GetLimit())
	now := s.now()
	var list []Task
	err = s.tx(ctx, func(tx pgx.Tx, _ db.Tenant) (err error) {
		list, err = s.st.list(ctx, tx, which, now, from, limit)
		return err
	})
	if err != nil {
		return nil, err
	}
	resp := &tasksv1.ListTasksResponse{Tasks: []*tasksv1.Task{}}
	if len(list) > limit {
		list = list[:limit]
		last := list[limit-1]
		resp.NextCursor = page.Encode(page.Key{Time: last.CreateTime, ID: last.ID})
	}
	for _, t := range list {
		resp.Tasks = append(resp.Tasks, toProto(t, now))
	}
	return resp, nil
}

// UpdateTask implements TaskService.UpdateTask.
func (s *TaskService) UpdateTask(ctx context.Context, req *tasksv1.UpdateTaskRequest) (*tasksv1.UpdateTaskResponse, error) {
	in := req.GetTask()
	id, ok := idOf(in.GetName())
	if !ok {
		return nil, taskNotFound(in.GetName())
	}
	change, err := changeOf(in, req.GetUpdateMask().GetPaths())
	if err != nil {
		return nil, apierr.New(connect.CodeInvalidArgument, "INVALID_UPDATE_MASK", "update_mask may name only title, notes and deadline_time")
	}
	now := s.now()
	var t Task
	err = s.tx(ctx, func(tx pgx.Tx, _ db.Tenant) error {
		var err error
		if t, err = s.st.get(ctx, tx, id, true); err != nil {
			return err
		}
		if err := t.Apply(change, now); err != nil {
			return err
		}
		return s.st.save(ctx, tx, t)
	})
	switch {
	case errors.Is(err, errNotFound):
		return nil, taskNotFound(in.GetName())
	case errors.Is(err, ErrTitleRequired), errors.Is(err, ErrTitleTooLong):
		return nil, apierr.New(connect.CodeInvalidArgument, "TITLE_REQUIRED", "a title has 1 to 200 characters")
	case errors.Is(err, ErrDoneImmutable):
		return nil, apierr.New(connect.CodeFailedPrecondition, "TASK_DONE_IMMUTABLE", "the task is done, so its deadline cannot change: reopen it first")
	case errors.Is(err, ErrDeadlineInPast):
		return nil, apierr.New(connect.CodeInvalidArgument, "DEADLINE_IN_PAST", "the deadline is in the past: set one that is still ahead")
	case err != nil:
		return nil, err
	}
	return &tasksv1.UpdateTaskResponse{Task: toProto(t, now)}, nil
}

// DeleteTask implements TaskService.DeleteTask. It announces the deletion
// after the commit, so a timer running on the task can stop.
func (s *TaskService) DeleteTask(ctx context.Context, req *tasksv1.DeleteTaskRequest) (*tasksv1.DeleteTaskResponse, error) {
	id, ok := idOf(req.GetName())
	if !ok {
		return nil, taskNotFound(req.GetName())
	}
	err := s.tx(ctx, func(tx pgx.Tx, _ db.Tenant) error {
		_, err := s.st.remove(ctx, tx, id, req.GetRequestId(), s.now())
		return err
	})
	if errors.Is(err, errNotFound) {
		return nil, taskNotFound(req.GetName())
	}
	if err != nil {
		return nil, err
	}
	// After the commit. A replayed delete announces it again: the bus may
	// not have taken the first event.
	if _, err := s.events.Publish(ctx, EventDeleted, namePrefix+id, idempotency(req.GetRequestId())...); err != nil {
		return nil, err
	}
	return &tasksv1.DeleteTaskResponse{}, nil
}

// CompleteTask implements TaskService.CompleteTask. It announces the
// completion after the commit, so a timer running on the task can stop.
func (s *TaskService) CompleteTask(ctx context.Context, req *tasksv1.CompleteTaskRequest) (*tasksv1.CompleteTaskResponse, error) {
	id, ok := idOf(req.GetName())
	if !ok {
		return nil, taskNotFound(req.GetName())
	}
	now := s.now()
	var t Task
	err := s.tx(ctx, func(tx pgx.Tx, _ db.Tenant) error {
		var err error
		if t, err = s.st.get(ctx, tx, id, true); err != nil {
			return err
		}
		replay, err := t.Complete(req.GetRequestId(), now)
		if err != nil || replay {
			return err
		}
		return s.st.save(ctx, tx, t)
	})
	switch {
	case errors.Is(err, errNotFound):
		return nil, taskNotFound(req.GetName())
	case errors.Is(err, ErrAlreadyDone):
		return nil, apierr.New(connect.CodeFailedPrecondition, "TASK_ALREADY_DONE", "the task is already done")
	case err != nil:
		return nil, err
	}
	if _, err := s.events.Publish(ctx, EventCompleted, namePrefix+id, idempotency(req.GetRequestId())...); err != nil {
		return nil, err
	}
	return &tasksv1.CompleteTaskResponse{Task: toProto(t, now)}, nil
}

// ReopenTask implements TaskService.ReopenTask.
func (s *TaskService) ReopenTask(ctx context.Context, req *tasksv1.ReopenTaskRequest) (*tasksv1.ReopenTaskResponse, error) {
	id, ok := idOf(req.GetName())
	if !ok {
		return nil, taskNotFound(req.GetName())
	}
	now := s.now()
	var t Task
	err := s.tx(ctx, func(tx pgx.Tx, _ db.Tenant) error {
		var err error
		if t, err = s.st.get(ctx, tx, id, true); err != nil {
			return err
		}
		if err := t.Reopen(now); err != nil {
			return err
		}
		return s.st.save(ctx, tx, t)
	})
	switch {
	case errors.Is(err, errNotFound):
		return nil, taskNotFound(req.GetName())
	case errors.Is(err, ErrNotDone):
		return nil, apierr.New(connect.CodeFailedPrecondition, "TASK_NOT_DONE", "the task is open: only a done task can be reopened")
	case err != nil:
		return nil, err
	}
	return &tasksv1.ReopenTaskResponse{Task: toProto(t, now)}, nil
}

func taskNotFound(name string) error {
	return apierr.New(connect.CodeNotFound, "TASK_NOT_FOUND", "no task named "+name)
}

func idempotency(requestID string) []events.Option {
	if requestID == "" {
		return nil
	}
	return []events.Option{events.IdempotencyKey(requestID)}
}

// idOf returns the ID of a task's resource name, tasks/{task}.
func idOf(name string) (string, bool) {
	id, ok := strings.CutPrefix(name, namePrefix)
	return id, ok && id != "" && !strings.Contains(id, "/")
}

// changeOf reads an update: the fields paths names, or every field the task
// sets when paths is empty.
func changeOf(in *tasksv1.Task, paths []string) (Change, error) {
	var c Change
	if len(paths) == 0 {
		if in.GetTitle() != "" {
			paths = append(paths, "title")
		}
		if in.GetNotes() != "" {
			paths = append(paths, "notes")
		}
		if in.GetDeadlineTime() != nil {
			paths = append(paths, "deadline_time")
		}
	}
	for _, p := range paths {
		switch p {
		case "title":
			title := in.GetTitle()
			c.Title = &title
		case "notes":
			notes := in.GetNotes()
			c.Notes = &notes
		case "deadline_time", "deadlineTime":
			c.SetDeadline = true
			c.Deadline = timePtr(in.GetDeadlineTime())
		default:
			return Change{}, errUnknownMaskPath
		}
	}
	return c, nil
}

func timePtr(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

func stamp(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

func toProto(t Task, now time.Time) *tasksv1.Task {
	state := tasksv1.Task_STATE_OPEN
	if t.State == Done {
		state = tasksv1.Task_STATE_DONE
	}
	return &tasksv1.Task{
		Name:         namePrefix + t.ID,
		Title:        t.Title,
		Notes:        t.Notes,
		State:        state,
		DeadlineTime: stamp(t.Deadline),
		Overdue:      t.Overdue(now),
		CreateTime:   timestamppb.New(t.CreateTime),
		UpdateTime:   timestamppb.New(t.UpdateTime),
		CompleteTime: stamp(t.CompleteTime),
	}
}
