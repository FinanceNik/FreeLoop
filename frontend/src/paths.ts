// Small path helpers for the display; the engine holds the real rules. A path can
// come from macOS or Linux (/Users/x/proj/src/a.ts) or from Windows
// (C:\Users\x\proj\src\a.ts, or with forward slashes), so every helper takes both
// separators.

// isAbsolute says whether a path starts at a root: /x, C:\x, C:/x or \\server\share.
export function isAbsolute(p: string): boolean {
  return p.startsWith("/") || /^[A-Za-z]:[\\/]/.test(p) || p.startsWith("\\\\");
}

// basename is the last part of a path.
export function basename(p: string): string {
  return p.split(/[\\/]/).filter(Boolean).pop() ?? p;
}

// nameStart is the index where the last part of a path begins.
export function nameStart(p: string): number {
  return Math.max(p.lastIndexOf("/"), p.lastIndexOf("\\")) + 1;
}

// windowsLike says whether a path is a Windows one, compared without regard to case.
function windowsLike(p: string): boolean {
  return /^[A-Za-z]:[\\/]/.test(p) || p.startsWith("\\\\");
}

// relative makes an agent's path relative to the project root, as git's are: with
// forward slashes, whichever the system writes.
export function relative(path: string, root: string): string {
  const win = windowsLike(root) || windowsLike(path);
  const slashed = (s: string) => (win ? s.replace(/\\/g, "/") : s);
  const p = slashed(path);
  const r0 = slashed(root);
  const r = r0.endsWith("/") ? r0 : r0 + "/";
  if (win ? p.toLowerCase().startsWith(r.toLowerCase()) : p.startsWith(r)) return p.slice(r.length);
  return path.startsWith("./") || path.startsWith(".\\") ? p.slice(2) : path;
}

// isMarkdownFile says whether the file viewer can show a file formatted (GUI-035).
export function isMarkdownFile(path: string): boolean {
  return /\.(md|markdown)$/i.test(path);
}

export function isPdfFile(path: string): boolean {
  return /\.pdf$/i.test(path);
}
