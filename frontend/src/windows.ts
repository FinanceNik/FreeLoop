// A session's own window (GUI-015) loads the page with ?session=<id>.

// windowSession returns the session a window shows on its own, from the
// page's query string, or null for the main window.
export function windowSession(search: string): string | null {
  const id = new URLSearchParams(search).get("session")?.trim();
  return id && /^[A-Za-z0-9_-]+$/.test(id) ? id : null;
}
