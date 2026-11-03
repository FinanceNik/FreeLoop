// Keeping the reader's place in the conversation.

// How near the end still counts as being at the end, in pixels.
const AT_THE_END = 60;

// holdsPlace says whether the conversation stops following what arrives, after a
// scroll event: yes once the reader scrolls up, and no again back at the end. It goes
// by the direction, because new output makes the end further away with no one having
// moved, and that must not stop it following.
export function holdsPlace(held: boolean, prevTop: number, top: number, toTheEnd: number): boolean {
  if (top < prevTop - 1) return true;
  return toTheEnd <= AT_THE_END ? false : held;
}

// heightAbove says how much the items that were wholly above the view grew: how far to
// scroll for what is in view to stay where it is.
export function heightAbove(items: readonly { bottom: number; before: number; after: number }[], viewTop: number): number {
  let grown = 0;
  for (const i of items) if (i.bottom <= viewTop) grown += i.after - i.before;
  return grown;
}

// Items drawn per frame. Each is drawn off screen, so this costs layout and no paint.
const BATCH = 30;

// drawOnce draws every child of box once, a batch per frame from the end up, for as
// long as it runs. Off-screen items have a placeholder height (app.css) until they are
// first drawn and WebKit has no scroll anchoring, so scrolling up made what is being
// read jump by as much as each item grew, up to 1680 px at a time. Once drawn, WebKit
// remembers an item's height, so those jumps end and the items again cost nothing
// off screen.
export function drawOnce(box: HTMLElement): () => void {
  const drawn = new WeakSet<Element>();
  let batch: HTMLElement[] = [];
  let frame = 0;
  const step = () => {
    frame = 0;
    for (const el of batch) el.style.contentVisibility = "";
    batch = [];
    for (let i = box.children.length - 1; i >= 0 && batch.length < BATCH; i--) {
      const el = box.children[i] as HTMLElement;
      if (drawn.has(el)) continue;
      drawn.add(el);
      batch.push(el);
    }
    if (batch.length === 0) return;
    const viewTop = box.getBoundingClientRect().top;
    const before = batch.map((el) => el.getBoundingClientRect());
    for (const el of batch) el.style.contentVisibility = "visible";
    const grown = heightAbove(
      batch.map((el, i) => ({ bottom: before[i].bottom, before: before[i].height, after: before[i].bottom <= viewTop ? el.getBoundingClientRect().height : before[i].height })),
      viewTop,
    );
    if (grown !== 0) box.scrollTop += grown;
    frame = requestAnimationFrame(step);
  };
  const start = () => {
    if (!frame) frame = requestAnimationFrame(step);
  };
  const added = new MutationObserver(start);
  added.observe(box, { childList: true });
  start();
  return () => {
    added.disconnect();
    if (frame) cancelAnimationFrame(frame);
    for (const el of batch) el.style.contentVisibility = "";
  };
}
