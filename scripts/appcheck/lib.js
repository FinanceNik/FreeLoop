// Helpers shared by the app check's scenarios (scenario.js, lived-in.js), put in front
// of each: the body of an async function run in the page, which returns one result per
// check.
const results = [];
const check = (name, ok, detail = "") => {
  results.push({ name, ok: !!ok, detail: String(detail) });
  // A picture of what failed, for a run no one watched (scripts/appcheck/drive.swift).
  if (!ok) window.webkit.messageHandlers.snap?.postMessage(name)?.catch(() => {});
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const until = async (what, fn, ms = 15000) => {
  const end = Date.now() + ms;
  for (;;) {
    const v = fn();
    if (v) return v;
    if (Date.now() > end) throw new Error(`timed out waiting for ${what}`);
    await sleep(50);
  }
};
const all = (sel, root = document) => [...root.querySelectorAll(sel)];
const withText = (sel, text, root = document) => all(sel, root).find((e) => e.textContent.includes(text));
// Icon-only controls are found by their accessible name, as VoiceOver finds them (GUI-041).
const named = (sel, name, root = document) => all(sel, root).find((e) => e.getAttribute("aria-label") === name);
const openSession = (title) => withText(".session-row", title).click();
const type = (box, text) => {
  Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value").set.call(box, text);
  box.dispatchEvent(new Event("input", { bubbles: true }));
};
const activePane = () => document.querySelector(".pane-active") ?? document;
const toTheEnd = (el) => el.scrollHeight - el.scrollTop - el.clientHeight;
const step = async (name, fn) => {
  try {
    await fn();
  } catch (e) {
    check(name, false, e.message);
  }
};
// One turn of the demo, approving whatever it asks. Returns what the sidebar said while
// a request waited.
const turn = async (title, afterSend = () => {}, text = "go") => {
  const pane = activePane();
  // A turn's end, not the notices and mode or model changes shown the same way.
  const ended = () => all(".turn-stop", pane).length;
  const before = ended();
  const box = await until("the message box", () => pane.querySelector(".composer textarea"));
  box.focus();
  type(box, text);
  await sleep(50);
  (await until("Send to be enabled", () => { const b = named(".composer-actions button.primary", "Send", pane); return b && !b.disabled && b; })).click();
  afterSend();
  let waitingLine = "";
  await until("the turn to end", () => {
    const allow = pane.querySelector(".approval-buttons button.primary");
    if (allow) {
      waitingLine = waitingLine || withText(".session-row", title)?.title || "";
      waitingDot = waitingDot || withText(".session-row", title)?.querySelector(".status-dot")?.getAttribute("aria-label") || "";
      allow.click();
    }
    return ended() > before;
  }, 30000);
  return waitingLine;
};
// The status dot a session's row showed while it waited for an approval, from the last turn.
let waitingDot = "";
// openPalette opens the command palette, or keeps it open, and types query into it.
const openPalette = async (query) => {
  if (!document.querySelector(".palette")) window.dispatchEvent(new KeyboardEvent("keydown", { key: "k", metaKey: true, bubbles: true }));
  const box = await until("the palette", () => document.querySelector(".palette input"), 4000);
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(box, query);
  box.dispatchEvent(new Event("input", { bubbles: true }));
  await sleep(100);
  return box;
};
