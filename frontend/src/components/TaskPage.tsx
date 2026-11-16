import { useCallback, useEffect, useState, type FormEvent } from "react";
import { board } from "../api/bff";
import { api, type TaskInput } from "../api/rest";
import type { EntryRow, TaskCard } from "../gen/commerce/todo/web/v1/board_pb";
import {
  durationSeconds,
  formatDateTime,
  formatDuration,
  formatTime,
  fromLocalInput,
  toLocalInput,
  tsDate,
} from "../format";

type Props = {
  task: string;
  reloadKey: number;
  onBack: () => void;
  onError: (err: unknown) => void;
  onClearError: () => void;
  onTimerChanged: () => Promise<void>;
};

export function TaskPage({ task, reloadKey, onBack, onError, onClearError, onTimerChanged }: Props) {
  const [card, setCard] = useState<TaskCard | undefined>(undefined);
  const [entries, setEntries] = useState<EntryRow[]>([]);
  const [entryCount, setEntryCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const res = await board.loadTaskPage({ task });
      setCard(res.task);
      setEntries(res.timeEntries);
      setEntryCount(res.entryCount);
    } catch (err) {
      onError(err);
    } finally {
      setLoading(false);
    }
  }, [task, onError]);

  useEffect(() => {
    void load();
  }, [load, reloadKey]);

  useEffect(() => {
    void onTimerChanged();
  }, [onTimerChanged]);

  /** Runs a command, then loads the page again. Returns whether it worked. */
  const run = async (command: () => Promise<unknown>, timer = false): Promise<boolean> => {
    setBusy(true);
    onClearError();
    let ok = true;
    try {
      await command();
    } catch (err) {
      onError(err);
      ok = false;
    } finally {
      setBusy(false);
    }
    await load();
    if (timer) await onTimerChanged();
    return ok;
  };

  if (!card) {
    return (
      <section>
        <BackButton onBack={onBack} />
        <p className="muted">{loading ? "Loading…" : "This task could not be loaded."}</p>
      </section>
    );
  }

  return (
    <section className="task-page">
      <BackButton onBack={onBack} />
      <div className="task-head">
        <h1>
          {card.title}
          {card.done && <span className="badge">Done</span>}
          {card.overdue && <span className="badge badge-danger">Overdue</span>}
        </h1>
        <div className="row">
          {!card.done && (
            <button type="button" className="btn" disabled={busy} onClick={() => run(() => api.startTimer(task), true)}>
              Start timer
            </button>
          )}
          <button
            type="button"
            className="btn"
            disabled={busy}
            onClick={() => run(() => (card.done ? api.reopenTask(task) : api.completeTask(task)))}
          >
            {card.done ? "Reopen" : "Complete"}
          </button>
          <button
            type="button"
            className="btn btn-danger-outline"
            disabled={busy}
            onClick={async () => {
              if (!window.confirm(`Delete "${card.title}"? This cannot be undone.`)) return;
              setBusy(true);
              onClearError();
              try {
                await api.deleteTask(task);
                await onTimerChanged();
                onBack();
              } catch (err) {
                onError(err);
                setBusy(false);
              }
            }}
          >
            Delete
          </button>
        </div>
      </div>

      <div className="stats">
        <div className="stat">
          <span className="stat-value">{formatDuration(durationSeconds(card.trackedDuration))}</span>
          <span className="muted">tracked</span>
        </div>
        <div className="stat">
          <span className="stat-value">{entryCount}</span>
          <span className="muted">{entryCount === 1 ? "time entry" : "time entries"}</span>
        </div>
        {tsDate(card.completeTime) && (
          <div className="stat">
            <span className="stat-value">{formatDateTime(tsDate(card.completeTime)!)}</span>
            <span className="muted">completed</span>
          </div>
        )}
      </div>

      <EditTask key={editKey(card)} card={card} busy={busy} onSave={(t, mask) => run(() => api.updateTask(task, t, mask))} />

      <h2>Time entries</h2>
      {entries.length === 0 ? (
        <p className="muted">No time recorded yet.</p>
      ) : (
        <>
          <table className="entries">
            <thead>
              <tr>
                <th>Start</th>
                <th>End</th>
                <th>Duration</th>
                <th>Source</th>
                <th aria-label="Actions" />
              </tr>
            </thead>
            <tbody>
              {entries.map((e) => (
                <EntryRowView
                  key={e.timeEntry}
                  entry={e}
                  busy={busy}
                  onDelete={() => {
                    if (window.confirm("Delete this time entry? This cannot be undone.")) {
                      void run(() => api.deleteTimeEntry(e.timeEntry));
                    }
                  }}
                />
              ))}
            </tbody>
          </table>
          {entryCount > entries.length && (
            <p className="muted small">
              Showing the latest {entries.length} of {entryCount} entries.
            </p>
          )}
        </>
      )}

      <RecordTime busy={busy} onRecord={(start, end) => run(() => api.createTimeEntry(task, start, end))} />
    </section>
  );
}

function BackButton({ onBack }: { onBack: () => void }) {
  return (
    <button type="button" className="btn-link back" onClick={onBack}>
      ← Back to the board
    </button>
  );
}

/** Resets the edit form whenever the saved task changes. */
function editKey(c: TaskCard): string {
  const d = tsDate(c.deadlineTime)?.getTime() ?? "";
  return `${c.title}\u0000${c.notes}\u0000${d}`;
}

function EditTask({
  card,
  busy,
  onSave,
}: {
  card: TaskCard;
  busy: boolean;
  onSave: (t: TaskInput, mask: (keyof TaskInput)[]) => Promise<boolean>;
}) {
  const initialDeadline = toLocalInput(tsDate(card.deadlineTime));
  const [title, setTitle] = useState(card.title);
  const [notes, setNotes] = useState(card.notes);
  const [deadline, setDeadline] = useState(initialDeadline);

  const mask: (keyof TaskInput)[] = [];
  if (title.trim() !== card.title) mask.push("title");
  if (notes !== card.notes) mask.push("notes");
  if (deadline !== initialDeadline) mask.push("deadlineTime");

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (mask.length === 0 || !title.trim()) return;
    // A deadline named in the mask but left out of the body is cleared.
    await onSave({ title: title.trim(), notes, deadlineTime: fromLocalInput(deadline) }, mask);
  };

  return (
    <form className="card edit-form" onSubmit={submit}>
      <label>
        <span className="muted">Title</span>
        <input value={title} maxLength={200} required onChange={(e) => setTitle(e.target.value)} />
      </label>
      <label>
        <span className="muted">Notes</span>
        <textarea rows={3} value={notes} onChange={(e) => setNotes(e.target.value)} />
      </label>
      <div className="row">
        <label className="inline-label">
          <span className="muted">Deadline</span>
          <input type="datetime-local" value={deadline} onChange={(e) => setDeadline(e.target.value)} />
        </label>
        {deadline && (
          <button type="button" className="btn-link" onClick={() => setDeadline("")}>
            Clear deadline
          </button>
        )}
        <span className="grow" />
        <button type="submit" className="btn btn-primary" disabled={busy || mask.length === 0 || !title.trim()}>
          Save changes
        </button>
      </div>
    </form>
  );
}

function EntryRowView({ entry, busy, onDelete }: { entry: EntryRow; busy: boolean; onDelete: () => void }) {
  const start = tsDate(entry.startTime);
  const end = tsDate(entry.endTime);
  const sameDay = start && end && start.toDateString() === end.toDateString();
  return (
    <tr>
      <td>{start ? formatDateTime(start) : "—"}</td>
      <td>{end ? (sameDay ? formatTime(end) : formatDateTime(end)) : "—"}</td>
      <td>{formatDuration(durationSeconds(entry.duration))}</td>
      <td>
        <span className={`badge ${entry.manual ? "" : "badge-accent"}`}>{entry.manual ? "Manual" : "Timer"}</span>
      </td>
      <td className="right">
        <button type="button" className="btn btn-small btn-danger-outline" disabled={busy} onClick={onDelete}>
          Delete
        </button>
      </td>
    </tr>
  );
}

function RecordTime({ busy, onRecord }: { busy: boolean; onRecord: (start: string, end: string) => Promise<boolean> }) {
  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [problem, setProblem] = useState("");

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const s = fromLocalInput(start);
    const en = fromLocalInput(end);
    if (!s || !en) {
      setProblem("Enter both a start and an end.");
      return;
    }
    if (new Date(en) <= new Date(s)) {
      setProblem("The end must be after the start.");
      return;
    }
    setProblem("");
    if (await onRecord(s, en)) {
      setStart("");
      setEnd("");
    }
  };

  return (
    <form className="card record-form" onSubmit={submit}>
      <h2>Record time by hand</h2>
      <div className="row">
        <label className="inline-label">
          <span className="muted">Start</span>
          <input type="datetime-local" value={start} required onChange={(e) => setStart(e.target.value)} />
        </label>
        <label className="inline-label">
          <span className="muted">End</span>
          <input type="datetime-local" value={end} required onChange={(e) => setEnd(e.target.value)} />
        </label>
        <span className="grow" />
        <button type="submit" className="btn btn-primary" disabled={busy}>
          Record
        </button>
      </div>
      {problem && <p className="field-error">{problem}</p>}
    </form>
  );
}
