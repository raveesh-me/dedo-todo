import { useEffect, useState } from "react";
import { api } from "../api/rest";
import type { RunningTimer } from "../gen/commerce/todo/web/v1/board_pb";
import { formatClock, tsDate } from "../format";

type Props = {
  timer: RunningTimer | undefined;
  onOpen: (task: string) => void;
  onStopped: () => void;
  onError: (err: unknown) => void;
};

export function TimerBar({ timer, onOpen, onStopped, onError }: Props) {
  const [now, setNow] = useState(() => Date.now());
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!timer) return;
    setNow(Date.now());
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, [timer]);

  if (!timer) {
    return <div className="timerbar timerbar-idle muted">No timer running</div>;
  }

  const start = tsDate(timer.startTime);
  const elapsed = start ? (now - start.getTime()) / 1000 : 0;

  const stop = async () => {
    setBusy(true);
    try {
      await api.stopTimer(timer.timer);
      onStopped();
    } catch (err) {
      onError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="timerbar" aria-live="polite">
      <span className="dot" aria-hidden="true" />
      <button type="button" className="btn-link timer-title" onClick={() => onOpen(timer.task)}>
        {timer.taskTitle || "A task not on this page"}
      </button>
      <span className="clock" aria-label="Elapsed time">
        {formatClock(elapsed)}
      </span>
      <button type="button" className="btn btn-danger btn-small" onClick={stop} disabled={busy}>
        Stop
      </button>
    </div>
  );
}
