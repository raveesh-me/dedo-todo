import { useState, type FormEvent } from "react";
import { api } from "../api/rest";
import { fromLocalInput } from "../format";

export function AddTaskForm({ onAdded, onError }: { onAdded: () => void; onError: (err: unknown) => void }) {
  const [title, setTitle] = useState("");
  const [notes, setNotes] = useState("");
  const [deadline, setDeadline] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!title.trim()) return;
    setBusy(true);
    try {
      await api.createTask({
        title: title.trim(),
        notes: notes.trim() || undefined,
        deadlineTime: fromLocalInput(deadline),
      });
      setTitle("");
      setNotes("");
      setDeadline("");
      onAdded();
    } catch (err) {
      onError(err);
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="card add-form" onSubmit={submit}>
      <div className="row">
        <input
          className="grow"
          placeholder="What needs doing?"
          aria-label="Title"
          value={title}
          maxLength={200}
          onChange={(e) => setTitle(e.target.value)}
          required
        />
        <label className="inline-label">
          <span className="muted">Deadline</span>
          <input type="datetime-local" value={deadline} onChange={(e) => setDeadline(e.target.value)} />
        </label>
      </div>
      <div className="row">
        <textarea
          className="grow"
          placeholder="Notes (optional)"
          aria-label="Notes"
          rows={2}
          value={notes}
          onChange={(e) => setNotes(e.target.value)}
        />
        <button type="submit" className="btn btn-primary" disabled={busy || !title.trim()}>
          Add task
        </button>
      </div>
    </form>
  );
}
