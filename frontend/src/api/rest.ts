// Commands go to the public Todo API over REST (docs/openapi.json).
import { authHeaders } from "../session";
import { AppError } from "./errors";

const BASE = "/commerce/todo/v1";

type Status = {
  code?: number;
  message?: string;
  details?: { "@type"?: string; reason?: string }[];
};

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { ...authHeaders(), Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  let res: Response;
  try {
    res = await fetch(`${BASE}/${path}`, {
      method,
      headers,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (err) {
    throw new AppError(`Could not reach the server: ${err instanceof Error ? err.message : String(err)}`);
  }
  const text = await res.text();
  let json: unknown = undefined;
  if (text) {
    try {
      json = JSON.parse(text);
    } catch {
      // Not JSON, such as a proxy's error page.
    }
  }
  if (!res.ok) {
    const s = (json ?? {}) as Status;
    const info = s.details?.find((d) => d["@type"] === "type.googleapis.com/google.rpc.ErrorInfo");
    throw new AppError(s.message || `${res.status} ${res.statusText}`.trim(), info?.reason ?? "", res.status);
  }
  return json as T;
}

/** A task as the API returns it (protojson, lowerCamelCase). */
export type Task = {
  name: string;
  title: string;
  notes?: string;
  deadlineTime?: string;
  state?: string;
};

export type TaskInput = { title?: string; notes?: string; deadlineTime?: string };

export type Timer = { name: string; task: string; startTime: string };

export type TimeEntry = {
  name: string;
  task: string;
  startTime: string;
  endTime: string;
  duration?: string;
  origin?: string;
};

export const api = {
  createTask: (t: TaskInput) => call<Task>("POST", "tasks", t),

  /**
   * Changes the fields named in mask. A field named but left out of t, such
   * as deadlineTime, is cleared.
   */
  updateTask: (task: string, t: TaskInput, mask: (keyof TaskInput)[]) =>
    call<Task>("PATCH", `${task}?updateMask=${encodeURIComponent(mask.join(","))}`, t),

  deleteTask: (task: string) => call<unknown>("DELETE", task),
  completeTask: (task: string) => call<Task>("POST", `${task}:complete`, {}),
  reopenTask: (task: string) => call<Task>("POST", `${task}:reopen`, {}),

  startTimer: (task: string) => call<Timer>("POST", "timers:startTimer", { task }),
  stopTimer: (timer: string) => call<TimeEntry>("POST", `${timer}:stop`, {}),

  createTimeEntry: (task: string, startTime: string, endTime: string) =>
    call<TimeEntry>("POST", "time-entries", { task, startTime, endTime }),
  deleteTimeEntry: (entry: string) => call<unknown>("DELETE", entry),
};
