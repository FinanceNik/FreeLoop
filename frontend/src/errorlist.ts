// The errors the window shows. Parallel sessions fail at the same time, so a new
// error must not replace one the user has not read, and each says which session
// it came from. Pure, so errorlist.test.ts covers it.

const KEEP = 4;

export function errorLine(text: string, sessionTitle?: string): string {
  return sessionTitle ? `${sessionTitle}: ${text}` : text;
}

export function addError(list: readonly string[], line: string): string[] {
  return [...list.filter((l) => l !== line), line].slice(-KEEP);
}
