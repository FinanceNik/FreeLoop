// Small path helpers for the display; the engine holds the real rules.

// basename is the last part of a path.
export function basename(p: string): string {
  return p.split("/").filter(Boolean).pop() ?? p;
}

// relative makes an agent's path relative to the project root, as git's are.
export function relative(path: string, root: string): string {
  const r = root.endsWith("/") ? root : root + "/";
  if (path.startsWith(r)) return path.slice(r.length);
  return path.replace(/^\.\//, "");
}
