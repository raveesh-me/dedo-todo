package app

import (
	"context"

	"connectrpc.com/connect/v2"

	tasksv1 "github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/tasks/v1"
	"github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/tasks/v1/tasksv1connect"
	"github.com/raveesh-me/dedo-todo/internal/time_tracking"
)

// tasksACL is the anti-corruption layer between the two modules: it answers
// Time Tracking's question about a task (time_tracking.Tasks) by calling
// Tasks' published contract, TaskService.GetTask, as the caller, and
// translates Tasks' model into Time Tracking's. The modules themselves never
// import each other: this is the one place that knows both, and the
// contract declares the call in (commerce.auth.v2.method).calls.
type tasksACL struct {
	client tasksv1connect.TaskServiceClient
}

var _ time_tracking.Tasks = tasksACL{}

// Open implements time_tracking.Tasks.
func (a tasksACL) Open(ctx context.Context, name string) (bool, error) {
	resp, err := a.client.GetTask(ctx, &tasksv1.GetTaskRequest{Name: name})
	switch connect.CodeOf(err) {
	case connect.CodeNotFound, connect.CodeInvalidArgument:
		return false, time_tracking.ErrTaskNotFound
	}
	if err != nil {
		return false, time_tracking.ErrTasksUnavailable
	}
	return resp.GetTask().GetState() == tasksv1.Task_STATE_OPEN, nil
}
