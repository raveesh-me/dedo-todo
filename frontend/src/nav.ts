// Views by URL hash: #/tasks/<id> is a task's page, anything else the board.

/** The task the hash opens, as a resource name, or "". */
export function taskFromHash(): string {
  const m = /^#\/tasks\/([a-z0-9-]+)$/.exec(window.location.hash);
  return m ? `tasks/${m[1]}` : "";
}

export function openTask(task: string): void {
  window.location.hash = `#/${task}`;
}

export function openBoard(): void {
  // Leaves "#" behind, which is the board too, without reloading the page.
  window.location.hash = "";
}
