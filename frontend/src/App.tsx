import { useCallback, useEffect, useState } from "react";
import { board, BUILD_SHA } from "./api/bff";
import { describe } from "./api/errors";
import { Board } from "./components/Board";
import { ErrorBanner } from "./components/ErrorBanner";
import { SessionPicker } from "./components/SessionPicker";
import { TaskPage } from "./components/TaskPage";
import { TimerBar } from "./components/TimerBar";
import { LoadBoardRequest_Show, type RunningTimer } from "./gen/commerce/todo/web/v1/board_pb";
import { openBoard, openTask, taskFromHash } from "./nav";
import { currentSession, setCurrentSession, type Session } from "./session";

export function App() {
  const [session, setSession] = useState<Session>(currentSession);
  const [task, setTask] = useState(taskFromHash);
  const [show, setShow] = useState(LoadBoardRequest_Show.UNSPECIFIED);
  const [timer, setTimer] = useState<RunningTimer | undefined>(undefined);
  // Bumped when something outside a view, such as the timer bar, changed
  // what it shows, so it loads again.
  const [reloadKey, setReloadKey] = useState(0);
  const [error, setError] = useState("");

  useEffect(() => {
    const onHash = () => {
      setTask(taskFromHash());
      setError("");
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  const report = useCallback((err: unknown) => setError(describe(err)), []);

  // The board's response carries the running timer; the task page asks for
  // it with a one-task board.
  const refreshTimer = useCallback(async () => {
    try {
      const res = await board.loadBoard({ limit: 1 });
      setTimer(res.runningTimer);
    } catch (err) {
      report(err);
    }
  }, [report]);

  const changeSession = (s: Session) => {
    setCurrentSession(s);
    setSession(s);
    setTimer(undefined);
    setError("");
  };

  const sessionKey = `${session.token}@${session.location}`;
  const shaOK = /^[0-9a-f]{12}$/.test(BUILD_SHA);

  return (
    <div className="app">
      <header className="topbar">
        <a
          className="brand"
          href="#"
          onClick={(e) => {
            e.preventDefault();
            openBoard();
          }}
        >
          Todo
        </a>
        <TimerBar
          timer={timer}
          onOpen={openTask}
          onStopped={() => {
            setTimer(undefined);
            setReloadKey((k) => k + 1);
          }}
          onError={report}
        />
        <SessionPicker session={session} onChange={changeSession} />
      </header>
      <main className="content">
        {!shaOK && <ErrorBanner message="This build has no valid VITE_BUILD_SHA, so it cannot reach its BFF." />}
        {error && <ErrorBanner message={error} onDismiss={() => setError("")} />}
        {task ? (
          <TaskPage
            key={`${sessionKey}|${task}`}
            task={task}
            reloadKey={reloadKey}
            onBack={openBoard}
            onError={report}
            onClearError={() => setError("")}
            onTimerChanged={refreshTimer}
          />
        ) : (
          <Board
            key={sessionKey}
            show={show}
            onShow={setShow}
            reloadKey={reloadKey}
            onTimer={setTimer}
            onOpen={openTask}
            onError={report}
            onClearError={() => setError("")}
          />
        )}
      </main>
      <footer className="footer">build {BUILD_SHA || "unknown"}</footer>
    </div>
  );
}
