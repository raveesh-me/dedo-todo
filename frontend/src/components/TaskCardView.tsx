import type { TaskCard } from "../gen/commerce/todo/web/v1/board_pb";
import { durationSeconds, formatDateTime, formatDuration, tsDate } from "../format";

type Props = {
  task: TaskCard;
  timing: boolean;
  busy: boolean;
  onOpen: () => void;
  onStart: () => void;
  onToggleDone: () => void;
  onDelete: () => void;
};

export function TaskCardView({ task, timing, busy, onOpen, onStart, onToggleDone, onDelete }: Props) {
  const deadline = tsDate(task.deadlineTime);
  const completed = tsDate(task.completeTime);
  return (
    <li className={`card task ${task.done ? "task-done" : ""} ${task.overdue ? "task-overdue" : ""}`}>
      <button type="button" className="task-main" onClick={onOpen}>
        <span className="task-title">
          {task.done && <span className="check" aria-label="Done">✓ </span>}
          {task.title}
        </span>
        <span className="task-meta">
          {deadline && (
            <span>
              Due {formatDateTime(deadline)}
              {task.overdue && <span className="badge badge-danger">Overdue</span>}
            </span>
          )}
          {completed && <span>Completed {formatDateTime(completed)}</span>}
          <span>Tracked {formatDuration(durationSeconds(task.trackedDuration))}</span>
          {timing && <span className="badge badge-accent">Timing</span>}
        </span>
      </button>
      <div className="task-actions">
        {!task.done && (
          <button type="button" className="btn btn-small" onClick={onStart} disabled={busy || timing}>
            Start timer
          </button>
        )}
        <button type="button" className="btn btn-small" onClick={onToggleDone} disabled={busy}>
          {task.done ? "Reopen" : "Complete"}
        </button>
        <button type="button" className="btn btn-small btn-danger-outline" onClick={onDelete} disabled={busy}>
          Delete
        </button>
      </div>
    </li>
  );
}
