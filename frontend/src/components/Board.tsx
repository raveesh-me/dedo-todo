import { useCallback, useEffect, useRef, useState } from "react";
import { board } from "../api/bff";
import { api } from "../api/rest";
import {
  LoadBoardRequest_Show as Show,
  type RunningTimer,
  type TaskCard,
} from "../gen/commerce/todo/web/v1/board_pb";
import { AddTaskForm } from "./AddTaskForm";
import { TaskCardView } from "./TaskCardView";

const TABS: { show: Show; label: string }[] = [
  { show: Show.UNSPECIFIED, label: "All" },
  { show: Show.OPEN, label: "Open" },
  { show: Show.DONE, label: "Done" },
  { show: Show.OVERDUE, label: "Overdue" },
];

const EMPTY: Record<Show, string> = {
  [Show.UNSPECIFIED]: "No tasks yet. Add one above.",
  [Show.OPEN]: "Nothing open.",
  [Show.DONE]: "Nothing done yet.",
  [Show.OVERDUE]: "Nothing overdue.",
};

type Props = {
  show: Show;
  onShow: (s: Show) => void;
  reloadKey: number;
  onTimer: (t: RunningTimer | undefined) => void;
  onOpen: (task: string) => void;
  onError: (err: unknown) => void;
  onClearError: () => void;
};

export function Board({ show, onShow, reloadKey, onTimer, onOpen, onError, onClearError }: Props) {
  const [tasks, setTasks] = useState<TaskCard[]>([]);
  const [cursor, setCursor] = useState("");
  const [timingTask, setTimingTask] = useState("");
  const [loading, setLoading] = useState(true);
  const [loaded, setLoaded] = useState(false);
  const [busyTask, setBusyTask] = useState("");
  // Only the latest load's response is shown, so a slow one for the
  // previous tab never overwrites the current tab.
  const seq = useRef(0);

  const load = useCallback(
    async (after = "") => {
      const n = ++seq.current;
      setLoading(true);
      try {
        const res = await board.loadBoard({ show, cursor: after });
        if (n !== seq.current) return;
        setTasks((prev) => (after ? [...prev, ...res.tasks] : res.tasks));
        setCursor(res.nextCursor);
        setTimingTask(res.runningTimer?.task ?? "");
        onTimer(res.runningTimer);
        setLoaded(true);
      } catch (err) {
        if (n === seq.current) onError(err);
      } finally {
        if (n === seq.current) setLoading(false);
      }
    },
    [show, onTimer, onError],
  );

  useEffect(() => {
    void load();
  }, [load, reloadKey]);

  /** Runs a command on a task, then loads the board again. */
  const run = async (task: string, command: () => Promise<unknown>) => {
    setBusyTask(task);
    onClearError();
    try {
      await command();
    } catch (err) {
      onError(err);
    } finally {
      setBusyTask("");
    }
    await load();
  };

  return (
    <section>
      <AddTaskForm
        onAdded={() => {
          onClearError();
          void load();
        }}
        onError={onError}
      />
      <nav className="tabs" role="tablist" aria-label="Show">
        {TABS.map((t) => (
          <button
            key={t.show}
            type="button"
            role="tab"
            aria-selected={show === t.show}
            className={`tab ${show === t.show ? "tab-active" : ""}`}
            onClick={() => onShow(t.show)}
          >
            {t.label}
          </button>
        ))}
      </nav>
      {loaded && tasks.length === 0 && !loading && <p className="empty muted">{EMPTY[show]}</p>}
      <ul className="tasks">
        {tasks.map((t) => (
          <TaskCardView
            key={t.task}
            task={t}
            timing={timingTask === t.task}
            busy={busyTask === t.task}
            onOpen={() => onOpen(t.task)}
            onStart={() => run(t.task, () => api.startTimer(t.task))}
            onToggleDone={() => run(t.task, () => (t.done ? api.reopenTask(t.task) : api.completeTask(t.task)))}
            onDelete={() => {
              if (window.confirm(`Delete "${t.title}"? This cannot be undone.`)) {
                void run(t.task, () => api.deleteTask(t.task));
              }
            }}
          />
        ))}
      </ul>
      <div className="more">
        {loading && <span className="muted">Loading…</span>}
        {!loading && cursor && (
          <button type="button" className="btn" onClick={() => load(cursor)}>
            Load more
          </button>
        )}
      </div>
    </section>
  );
}
