// What "Copy a report" puts on the clipboard for a pane or page that failed to draw: the
// engine's problem report, when it could be had, with the window's own error under it.

const MAX_STACK = 8000;

export function errorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  if (typeof err === "string") return err;
  try {
    return JSON.stringify(err) ?? String(err);
  } catch {
    return String(err);
  }
}

export function failureReport(report: string, err: unknown, componentStack: string): string {
  const head = err instanceof Error ? `${err.name}: ${err.message}` : errorMessage(err);
  const stack = (err instanceof Error ? (err.stack ?? "") : "") + componentStack;
  const cut = stack.length > MAX_STACK ? `${stack.slice(0, MAX_STACK)}\n…` : stack;
  const sep = report && !report.endsWith("\n") ? "\n" : "";
  return [report + sep, "", ["window-error", head].join(" "), cut, ""].join("\n");
}
