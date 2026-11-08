// What the provider form warns about (a saved key is tied to its address).

function parse(raw: string): URL | null {
  try {
    return new URL(raw.trim());
  } catch {
    return null;
  }
}

// keyMoves reports a change of address that would send a saved key elsewhere. An
// empty address means the provider's default.
export function keyMoves(before: string, after: string): boolean {
  if (before.trim() === after.trim()) return false;
  const a = parse(before);
  const b = parse(after);
  return !a || !b || a.origin !== b.origin;
}

// plainRemote reports an unencrypted address off this computer.
export function plainRemote(raw: string): boolean {
  const u = parse(raw);
  if (!u || u.protocol !== "http:") return false;
  const h = u.hostname;
  return h !== "localhost" && !h.endsWith(".localhost") && !h.startsWith("127.") && h !== "[::1]";
}
