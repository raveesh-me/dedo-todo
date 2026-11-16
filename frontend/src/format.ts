import { timestampDate, type Duration, type Timestamp } from "@bufbuild/protobuf/wkt";

/** Whole seconds of a protobuf Duration. */
export function durationSeconds(d: Duration | undefined): number {
  if (!d) return 0;
  return Number(d.seconds) + Math.round(d.nanos / 1e9);
}

/** Tracked time, such as "1h 23m", "12m" or "40s". */
export function formatDuration(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return `${s % 60}s`;
}

/** A running timer's elapsed time, such as "1:02:03" or "02:03". */
export function formatClock(totalSeconds: number): string {
  const s = Math.max(0, Math.floor(totalSeconds));
  const h = Math.floor(s / 3600);
  const mm = String(Math.floor((s % 3600) / 60)).padStart(2, "0");
  const ss = String(s % 60).padStart(2, "0");
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

const dateTime = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });
const timeOnly = new Intl.DateTimeFormat(undefined, { timeStyle: "short" });

export function formatDateTime(d: Date): string {
  return dateTime.format(d);
}

export function formatTime(d: Date): string {
  return timeOnly.format(d);
}

export function tsDate(t: Timestamp | undefined): Date | undefined {
  return t ? timestampDate(t) : undefined;
}

/** A date as a datetime-local input's value, in local time. */
export function toLocalInput(d: Date | undefined): string {
  if (!d) return "";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

/** A datetime-local input's value (local time) as RFC 3339, or undefined. */
export function fromLocalInput(v: string): string | undefined {
  if (!v) return undefined;
  const d = new Date(v);
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
}

/** The resource ID, the last segment of a resource name. */
export function resourceId(name: string): string {
  return name.slice(name.lastIndexOf("/") + 1);
}
