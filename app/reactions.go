package app

import (
	"context"
	"log/slog"

	"connectrpc.com/connect/v2"
	"github.com/GoHighLevel/dedo/events/driver"

	time_trackingv1 "github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/time_tracking/v1"
	"github.com/raveesh-me/dedo-todo/gen/go/commerce/todo/time_tracking/v1/time_trackingv1connect"
	"github.com/raveesh-me/dedo-todo/internal/tasks"
)

// bus is the service's event bus driver: it logs every event Tasks
// publishes, as a record of what was announced, and hands it to the
// reactions that orchestrate across modules. It acknowledges an event only
// once its reaction has run, so an event is never lost: a reaction that
// fails makes Publish retry, and then the command fail with
// EVENT_NOT_PUBLISHED, which its caller retries with the same request_id.
//
// In this process the reactions run as the caller who caused the event,
// with the caller's token forwarded, as commerce requires of every call. A
// deployment with a broker swaps this driver for the broker's; the
// reactions then move to an orchestrator of their own, which calls the
// same API.
type bus struct {
	log   *slog.Logger
	react func(context.Context, driver.Event) error
}

func newBus(log *slog.Logger, r *reactions) *bus {
	return &bus{log: log, react: r.handle}
}

// Publish implements driver.Driver.
func (b *bus) Publish(ctx context.Context, e driver.Event) error {
	b.log.InfoContext(ctx, "event", "type", e.Type, "subject", e.Subject, "id", e.ID, "idempotency_key", e.IdempotencyKey)
	return b.react(ctx, e)
}

// reactions keep tasks and timers consistent: a task that is completed or
// deleted has no timer left running on it.
type reactions struct {
	timers time_trackingv1connect.TimerServiceClient
	log    *slog.Logger
}

func newReactions(timers time_trackingv1connect.TimerServiceClient, log *slog.Logger) *reactions {
	return &reactions{timers: timers, log: log}
}

// handle stops the caller's running timer when it is timing the task the
// event is about, which writes its one time entry. It is idempotent: a
// redelivered event finds no timer on the task and does nothing.
func (r *reactions) handle(ctx context.Context, e driver.Event) error {
	switch e.Type {
	case tasks.EventCompleted, tasks.EventDeleted:
	default:
		return nil
	}
	list, err := r.timers.ListTimers(ctx, &time_trackingv1.ListTimersRequest{})
	if connect.CodeOf(err) == connect.CodePermissionDenied {
		// The caller may not track time, so has no timer of theirs to stop.
		return nil
	}
	if err != nil {
		return err
	}
	for _, t := range list.GetTimers() {
		if t.GetTask() != e.Subject {
			continue
		}
		_, err := r.timers.StopTimer(ctx, &time_trackingv1.StopTimerRequest{Name: t.GetName()})
		if err != nil && connect.CodeOf(err) != connect.CodeNotFound { // stopped meanwhile: done
			return err
		}
		r.log.InfoContext(ctx, "stopped the timer on a task that is no longer open", "task", e.Subject, "timer", t.GetName(), "event", e.Type)
	}
	return nil
}
