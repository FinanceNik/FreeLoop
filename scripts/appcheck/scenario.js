// The body of an async function run in the page; it returns one result per step.
const results = [];
const check = (name, ok, detail = "") => results.push({ name, ok: !!ok, detail: String(detail) });
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
const turn = async (title, afterSend = () => {}) => {
  const pane = activePane();
  const ended = () => all(".turn-end", pane).length;
  const before = ended();
  const box = await until("the message box", () => pane.querySelector(".composer textarea"));
  box.focus();
  type(box, "go");
  await sleep(50);
  (await until("Send to be enabled", () => { const b = withText(".composer-actions button.primary", "Send", pane); return b && !b.disabled && b; })).click();
  afterSend();
  let waitingLine = "";
  await until("the turn to end", () => {
    const allow = pane.querySelector(".approval-buttons button.primary");
    if (allow) {
      waitingLine = waitingLine || withText(".session-row", title)?.querySelector(".session-activity")?.textContent || "";
      allow.click();
    }
    return ended() > before;
  }, 30000);
  return waitingLine;
};

await until("the session list", () => withText(".session-row", "Check alpha"));

// Opening a long transcript lands on its end and stays there while it settles.
await step("a long transcript opens at its end", async () => {
  openSession("Check long");
  const c = await until("the conversation", () => { const x = document.querySelector(".conversation"); return x && x.scrollHeight > x.clientHeight && x; });
  const seen = [];
  for (let i = 0; i < 30; i++) {
    await sleep(100);
    seen.push(Math.round(toTheEnd(c)));
  }
  check("a long transcript opens at its end", toTheEnd(c) <= 60, `${c.children.length} items, px from the end over 3 s: ${seen.filter((_, i) => i % 5 === 0).join(" ")} ... ${seen.at(-1)}`);
});

const conversation = () => activePane().querySelector(".conversation");
const atEnd = async (c) => { c.scrollTop = c.scrollHeight; await sleep(150); };
const taller = async () => {
  const last = [...conversation().children].filter((e) => e.getBoundingClientRect().height > 0).at(-1);
  last.style.paddingBottom = `${(parseInt(last.style.paddingBottom) || 0) + 400}px`;
  await sleep(200);
};

await step("content growing with no event is followed", async () => {
  const c = conversation();
  await atEnd(c);
  await taller();
  check("content growing with no event is followed", toTheEnd(c) <= 60, `${Math.round(toTheEnd(c))} px from the end`);
});

await step("a wheel twitch that scrolls nothing does not stop the following", async () => {
  const c = conversation();
  await atEnd(c);
  c.dispatchEvent(new WheelEvent("wheel", { deltaY: -0.5, bubbles: true }));
  await sleep(700);
  await taller();
  check("a wheel twitch that scrolls nothing does not stop the following", toTheEnd(c) <= 60, `${Math.round(toTheEnd(c))} px from the end`);
});

await step("scrolling inside a code block does not stop the following", async () => {
  const c = conversation();
  await atEnd(c);
  const inner = document.createElement("pre");
  inner.style.cssText = "height:60px;overflow:auto";
  inner.textContent = "x\n".repeat(50);
  c.appendChild(inner);
  await atEnd(c);
  inner.scrollTop = 200;
  inner.dispatchEvent(new WheelEvent("wheel", { deltaY: -40, bubbles: true }));
  inner.scrollTop = 160;
  await sleep(700);
  inner.remove();
  await sleep(100);
  await atEnd(c);
  await taller();
  check("scrolling inside a code block does not stop the following", toTheEnd(c) <= 60, `${Math.round(toTheEnd(c))} px from the end`);
});

await step("a small nudge near the end keeps following", async () => {
  const c = conversation();
  await atEnd(c);
  c.dispatchEvent(new WheelEvent("wheel", { deltaY: -20, bubbles: true }));
  c.scrollTop -= 20;
  await sleep(700);
  await taller();
  check("a small nudge near the end keeps following", toTheEnd(c) <= 60, `${Math.round(toTheEnd(c))} px from the end`);
});

await step("scrolling well up holds through new content, and the end resumes", async () => {
  const c = conversation();
  await atEnd(c);
  c.dispatchEvent(new WheelEvent("wheel", { deltaY: -600, bubbles: true }));
  c.scrollTop -= 600;
  await sleep(700);
  const top = c.scrollTop;
  await taller();
  check("scrolling well up holds through new content", Math.abs(c.scrollTop - top) <= 1, `${Math.round(top)} became ${Math.round(c.scrollTop)}`);
  c.dispatchEvent(new WheelEvent("wheel", { deltaY: 5000, bubbles: true }));
  c.scrollTop = c.scrollHeight;
  await sleep(150);
  await taller();
  check("back at the end it follows again", toTheEnd(c) <= 60, `${Math.round(toTheEnd(c))} px from the end`);
});

await step("opening a tool card to read it is not pulled away", async () => {
  const c = conversation();
  await atEnd(c);
  const card = all("details.tool", c).filter((d) => !d.open).at(-1) ?? all("summary", c).at(-1)?.parentElement;
  if (!card) throw new Error("no card to open");
  card.querySelector("summary").click();
  await sleep(200);
  const top = card.getBoundingClientRect().top;
  await taller();
  check("opening a tool card to read it is not pulled away", Math.abs(card.getBoundingClientRect().top - top) <= 1, `moved ${Math.round(card.getBoundingClientRect().top - top)} px`);
  card.querySelector("summary").click();
  await atEnd(c);
});

// While a turn streams, each frame's distance from the end shows any lag.
await step("a running turn stays at the end, frame by frame", async () => {
  const c = conversation();
  await atEnd(c);
  const seen = [];
  let on = true;
  const watch = () => { seen.push(toTheEnd(c)); if (on) requestAnimationFrame(watch); };
  requestAnimationFrame(watch);
  await turn("Check long");
  await sleep(200);
  on = false;
  const far = seen.filter((d) => d > 60).length;
  check("a running turn stays at the end, frame by frame", toTheEnd(c) <= 60 && far <= seen.length * 0.05, `${far} of ${seen.length} frames over 60 px, worst ${Math.round(Math.max(...seen))}, at the end ${Math.round(toTheEnd(c))}`);
});

await step("a session opens", async () => {
  openSession("Check alpha");
  await until("the conversation", () => document.querySelector(".conversation"));
  check("a session opens", true);
});

await step("a turn runs to its end, with approvals", async () => {
  const line = await turn("Check alpha");
  check("a turn runs to its end, with approvals", true);
  check("the sidebar names what a waiting session asks", line.startsWith("Needs you: "), `it said "${line}"`);
});

await step("the view follows a running agent", async () => {
  // Enough to overflow, and cards collapse along the way.
  await turn("Check alpha");
  await turn("Check alpha");
  await turn("Check alpha");
  await sleep(300);
  const c = activePane().querySelector(".conversation");
  check("the conversation has outgrown the view", c.scrollHeight > c.clientHeight, `${c.scrollHeight} against ${c.clientHeight}`);
  check("the view follows a running agent", toTheEnd(c) <= 60, `${Math.round(toTheEnd(c))} px from the end`);
});

await step("the view moving by itself does not stop the following", async () => {
  // Cards collapsing at the end shorten the conversation with no one touching it, and
  // the view lands on the new end. Then it has to go on following.
  const c = activePane().querySelector(".conversation");
  const tall = document.createElement("div");
  tall.style.height = "1500px";
  c.appendChild(tall);
  c.scrollTop = c.scrollHeight;
  await sleep(200);
  tall.remove();
  await sleep(200);
  const box = activePane().querySelector(".composer textarea");
  type(box, "a\nb\nc\nd\ne\nf");
  await sleep(300);
  check("the view moving by itself does not stop the following", toTheEnd(c) <= 60, `${Math.round(toTheEnd(c))} px from the end`);
  type(box, "");
  await sleep(200);
  c.scrollTop = c.scrollHeight;
  await sleep(100);
});

await step("scrolling up holds the place", async () => {
  const c = activePane().querySelector(".conversation");
  c.dispatchEvent(new WheelEvent("wheel", { deltaY: -200, bubbles: true }));
  c.scrollTop = Math.max(0, c.scrollTop - 200);
  await sleep(600);
  const top = c.scrollTop;
  // A taller message box resizes the conversation, which pulls a following view to the end.
  const box = activePane().querySelector(".composer textarea");
  type(box, "a\nb\nc\nd\ne\nf");
  await sleep(300);
  check("scrolling up holds the place", Math.abs(c.scrollTop - top) <= 1, `${top} became ${c.scrollTop}`);
  c.scrollTop = c.scrollHeight;
  await sleep(100);
  type(box, "");
});

await step("a finished session says so on the board", async () => {
  withText(".footer-links button", "Board").click();
  const card = await until("the board's card", () => withText(".board-card", "Check alpha"));
  const line = card.querySelector(".session-activity")?.textContent ?? "";
  check("a finished session says so on the board", line.startsWith("Finished"), `it said "${line}"`);
  openSession("Check alpha");
  await until("the conversation", () => document.querySelector(".conversation"));
});

await step("panes fit, and the keyboard moves between them", async () => {
  document.querySelector('.layout-switch button[aria-label="2 panes"]').click();
  (await until("an empty pane's offer", () => withText(".pane-offer", "Check beta"))).click();
  await until("two sessions in panes", () => all(".pane .session-header").length === 2);
  await sleep(300);
  const over = all(".pane").flatMap((p) => {
    const right = p.getBoundingClientRect().right;
    return all("*", p).filter((e) => !e.closest(".export-options") && e.getBoundingClientRect().width > 0 && e.getBoundingClientRect().right > right + 1);
  });
  check("panes fit", over.length === 0, `${over.length} elements past a pane, such as ${over[0]?.className}`);
  const before = document.querySelector(".pane-active")?.dataset.position;
  window.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowLeft", ctrlKey: true, shiftKey: true, bubbles: true }));
  await sleep(200);
  const after = document.querySelector(".pane-active")?.dataset.position;
  check("the keyboard moves between panes", before !== after, `stayed on ${before}`);
  // Inactive panes keep only the input.
  const hidden = getComputedStyle(document.querySelector(".pane-split:not(.pane-active) .composer-actions")).display === "none";
  check("a pane the user is not in hides its controls", hidden);
  // A turn in a pane the user has left goes on being followed.
  const running = document.querySelector(".pane-active");
  const other = all(".pane").find((p) => p !== running);
  await turn(running.querySelector("h2").textContent, () => other.querySelector("textarea").focus());
  await sleep(300);
  const c = running.querySelector(".conversation");
  check("a pane the user left keeps following", toTheEnd(c) <= 60, `${Math.round(toTheEnd(c))} px from the end`);
  document.querySelector('.layout-switch button[aria-label="One pane"]').click();
  await sleep(200);
});

await step("the bypass setting", async () => {
  document.querySelector(".settings-link").click();
  (await until("the General page", () => withText(".settings-nav button", "General"))).click();
  const box = () => withText("label", "Allow bypass")?.querySelector("input");
  await until("the setting", box);
  // A served build has no native dialogs, so this answers that one call.
  let answer = "Cancel";
  let asked = 0;
  const real = window.fetch;
  window.fetch = (u, o) => {
    if (`${u}${o?.body ?? ""}`.includes("Allow%20bypass") || `${u}${o?.body ?? ""}`.includes("Allow bypass")) {
      asked++;
      return Promise.resolve(new Response(JSON.stringify(answer), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    return real(u, o);
  };
  box().click();
  await sleep(800);
  check("the setting asks first", asked === 1, `asked ${asked} times`);
  check("declining leaves it off", !box().checked);
  answer = "OK";
  box().click();
  await until("the tick", () => box().checked, 3000);
  await sleep(800);
  withText(".settings-nav button", "Providers").click();
  await sleep(300);
  withText(".settings-nav button", "General").click();
  await until("the setting", box);
  check("the setting survives leaving the page", box().checked);
  window.fetch = real;
});

return results;
