// A click clears a selection anywhere in the window. WebKit does so only for a click on
// text it could select, so after ⌘A a click on a button, a sidebar row or empty space
// left everything selected. Pure, so selection.test.ts covers the decision.

export interface ClickOnSelection {
  // A selection was there when the button went down.
  had: boolean;
  // Which click of a double or triple click this is.
  clicks: number;
  // How far the pointer moved, in pixels: a drag selects, or moves what is selected.
  moved: number;
  shift: boolean;
  // Text fields and the terminal keep selections of their own.
  ownSelection: boolean;
}

export function clearsSelection(c: ClickOnSelection): boolean {
  return c.had && c.clicks === 1 && c.moved < 4 && !c.shift && !c.ownSelection;
}

// clearSelectionOnClick listens on window until the returned function is called.
export function clearSelectionOnClick(): () => void {
  let down: { x: number; y: number; had: boolean } | null = null;
  const onDown = (e: MouseEvent) => {
    if (e.button !== 0) return;
    const selection = getSelection();
    down = { x: e.clientX, y: e.clientY, had: !!selection && !selection.isCollapsed };
  };
  const onUp = (e: MouseEvent) => {
    if (e.button !== 0 || !down) return;
    const target = e.target instanceof Element ? e.target : null;
    const go = clearsSelection({
      had: down.had,
      clicks: e.detail,
      moved: Math.hypot(e.clientX - down.x, e.clientY - down.y),
      shift: e.shiftKey,
      ownSelection: !!target?.closest('input, textarea, [contenteditable="true"], .xterm'),
    });
    down = null;
    if (go) getSelection()?.removeAllRanges();
  };
  window.addEventListener("mousedown", onDown, true);
  window.addEventListener("mouseup", onUp, true);
  return () => {
    window.removeEventListener("mousedown", onDown, true);
    window.removeEventListener("mouseup", onUp, true);
  };
}
