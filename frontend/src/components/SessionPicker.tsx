import { ACCOUNTS, type Session } from "../session";

export function SessionPicker({ session, onChange }: { session: Session; onChange: (s: Session) => void }) {
  const account = ACCOUNTS.find((a) => a.token === session.token) ?? ACCOUNTS[0];
  return (
    <div className="session">
      <span className="muted">Signed in as</span>
      <select
        aria-label="User"
        value={session.token}
        onChange={(e) => {
          const next = ACCOUNTS.find((a) => a.token === e.target.value) ?? ACCOUNTS[0];
          const location = next.locations.includes(session.location) ? session.location : next.locations[0];
          onChange({ token: next.token, location });
        }}
      >
        {ACCOUNTS.map((a) => (
          <option key={a.token} value={a.token}>
            {a.label}
          </option>
        ))}
      </select>
      <span className="muted">at</span>
      <select
        aria-label="Location"
        value={session.location}
        onChange={(e) => onChange({ token: session.token, location: e.target.value })}
      >
        {account.locations.map((l) => (
          <option key={l} value={l}>
            {l}
          </option>
        ))}
      </select>
    </div>
  );
}
