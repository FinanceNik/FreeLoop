// Where a context menu opens: at the pointer, moved in so all of it can be seen.
export function menuPosition(x: number, y: number, width: number, height: number, viewWidth: number, viewHeight: number): { left: number; top: number } {
  const margin = 8;
  return {
    left: Math.max(margin, Math.min(x, viewWidth - width - margin)),
    top: Math.max(margin, Math.min(y, viewHeight - height - margin)),
  };
}

// nextMenuItem says which item a key moves to, wrapping around, or -1 for another key.
export function nextMenuItem(key: string, at: number, count: number): number {
  if (count === 0) return -1;
  switch (key) {
    case "ArrowDown":
      return (at + 1) % count;
    case "ArrowUp":
      return (at - 1 + count) % count;
    case "Home":
      return 0;
    case "End":
      return count - 1;
    default:
      return -1;
  }
}
