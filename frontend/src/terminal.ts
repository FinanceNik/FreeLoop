// Helpers for the integrated terminal (GUI-012), kept apart from the view so
// they can be tested without a DOM.

// bytesOf decodes a terminal chunk, which arrives base64-encoded because
// screen output is bytes, not always whole UTF-8 characters.
export function bytesOf(data: string | null | undefined): Uint8Array {
  if (!data) return new Uint8Array();
  const bin = atob(data);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

// fitSize is how many columns and rows of cells fit in a box, at least
// 2×1 and at most what the engine accepts.
export function fitSize(width: number, height: number, cellWidth: number, cellHeight: number): { cols: number; rows: number } {
  if (!(cellWidth > 0) || !(cellHeight > 0)) return { cols: 80, rows: 24 };
  return {
    cols: Math.min(1000, Math.max(2, Math.floor(width / cellWidth))),
    rows: Math.min(500, Math.max(1, Math.floor(height / cellHeight))),
  };
}

// after returns the part of a chunk printed at or after byte end of the
// terminal's output, given the chunk's offset: what a view that already
// shows everything up to end still needs.
export function after(data: Uint8Array, offset: number, end: number): Uint8Array {
  if (offset >= end) return data;
  return data.subarray(Math.min(data.length, end - offset));
}
