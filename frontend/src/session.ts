// Who the app acts as: a dev token and the location it acts in, sent on
// every request and remembered in localStorage.

export type Session = { token: string; location: string };

/** The dev tokens and the locations each may act in. */
export const ACCOUNTS: { token: string; label: string; locations: string[] }[] = [
  { token: "alice", label: "alice", locations: ["loc-demo", "loc-other"] },
  { token: "bob", label: "bob", locations: ["loc-demo"] },
  { token: "reader", label: "reader (read-only)", locations: ["loc-demo"] },
];

export const DEFAULT_SESSION: Session = { token: "alice", location: "loc-demo" };

const KEY = "dedo-todo.session";

export function loadSession(): Session {
  try {
    const raw = window.localStorage.getItem(KEY);
    if (raw) {
      const s = JSON.parse(raw) as Partial<Session>;
      const account = ACCOUNTS.find((a) => a.token === s.token);
      if (account && typeof s.location === "string" && account.locations.includes(s.location)) {
        return { token: account.token, location: s.location };
      }
    }
  } catch {
    // Storage may be unavailable or hold something else: use the default.
  }
  return DEFAULT_SESSION;
}

export function saveSession(s: Session): void {
  try {
    window.localStorage.setItem(KEY, JSON.stringify(s));
  } catch {
    // Not remembered; the session still applies until the page reloads.
  }
}

let current: Session = loadSession();

/** The session requests are sent with now. */
export function currentSession(): Session {
  return current;
}

export function setCurrentSession(s: Session): void {
  current = s;
  saveSession(s);
}

/** The headers every request, to the BFF or the API, must carry. */
export function authHeaders(): Record<string, string> {
  return {
    Authorization: `Bearer ${current.token}`,
    "location-id": current.location,
    Version: "v4",
  };
}
