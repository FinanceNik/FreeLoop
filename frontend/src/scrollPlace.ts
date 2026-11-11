// Keeping the reader's place in the conversation.

// HOLD_PLACE, sent to the conversation, stops the following: something showed the user a
// place in it, and the next event must not pull them away.
export const HOLD_PLACE = "freeloop-hold-place";

// How near the end still counts as being at the end, in pixels.
const AT_THE_END = 60;

// holdsPlace says whether the conversation stops following what arrives, after a
// scroll event: yes once the reader has scrolled up away from the end, and no again back
// near it. A nudge that stays near the end does not count, since a trackpad twitches.
// Only the reader's own input counts (byReader). The view also moves up by itself: cards
// collapsing at the end shorten the conversation and the view lands on the new end,
// which in a real WebKit view stopped the following for good. New output makes the end
// further away with no one having moved, and that must not stop it either.
export function holdsPlace(held: boolean, prevTop: number, top: number, toTheEnd: number, byReader: boolean): boolean {
  if (toTheEnd <= AT_THE_END) return false;
  return byReader && top < prevTop - 1 ? true : held;
}

// settledPlace says whether the place is held once the reader's input has ended, for
// input that may have moved nothing: wheel events over a code block scroll that block,
// and a twitch scrolls nothing at all. It goes by where the view actually is.
export function settledPlace(held: boolean, toTheEnd: number): boolean {
  return toTheEnd <= AT_THE_END ? false : held;
}

// How long after the reader's last input a scroll is still theirs, in milliseconds:
// the keyboard's scrolling is animated. A trackpad's momentum goes on sending wheel
// events, so it stays the reader's for as long as it moves.
const READER_INPUT_LASTS = 400;

// readerScrolling says whether a scroll now comes from the reader.
export function readerScrolling(now: number, lastInput: number, pointerDown: boolean): boolean {
  return pointerDown || now - lastInput <= READER_INPUT_LASTS;
}

// READER_PAUSE is how long new content leaves the view alone after the reader moved up,
// so a first small step is not pulled back before the next one arrives.
export const READER_PAUSE = READER_INPUT_LASTS;

// The keys that move a page up.
export function scrollsUpByKey(key: string): boolean {
  return ["ArrowUp", "PageUp", "Home"].includes(key);
}

// The keys that scroll a page.
export function scrollsByKey(key: string): boolean {
  return ["ArrowUp", "ArrowDown", "PageUp", "PageDown", "Home", "End", " "].includes(key);
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
    const top = box.scrollTop;
    const before = batch.map((el) => el.getBoundingClientRect());
    for (const el of batch) el.style.contentVisibility = "visible";
    const grown = heightAbove(
      batch.map((el, i) => ({ bottom: before[i].bottom, before: before[i].height, after: before[i].bottom <= viewTop ? el.getBoundingClientRect().height : before[i].height })),
      viewTop,
    );
    // From the place before, not the one now: at the end, a conversation that got shorter
    // has already moved the view up by as much, and moving it again showed a place where
    // nothing is drawn, so the view went blank for a frame in every such batch.
    if (grown !== 0) box.scrollTop = top + grown;
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
