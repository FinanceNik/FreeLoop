// The body of an async function run in the page; it returns one result per step.
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
const turn = async (title, afterSend = () => {}) => {
  const pane = activePane();
  const ended = () => all(".turn-end", pane).length;
  const before = ended();
  const box = await until("the message box", () => pane.querySelector(".composer textarea"));
  box.focus();
  type(box, "go");
  await sleep(50);
  (await until("Send to be enabled", () => { const b = named(".composer-actions button.primary", "Send", pane); return b && !b.disabled && b; })).click();
  afterSend();
  let waitingLine = "";
  await until("the turn to end", () => {
    const allow = pane.querySelector(".approval-buttons button.primary");
    if (allow) {
      waitingLine = waitingLine || withText(".session-row", title)?.title || "";
      allow.click();
    }
    return ended() > before;
  }, 30000);
  return waitingLine;
};

await until("the session list", () => withText(".session-row", "Check alpha"));
check("the sidebar shows the mark", all(".sidebar h1 svg.logo circle").length === 3);

await step("a session shows what it runs on", async () => {
  const mark = (title) => withText(".session-row", title)?.querySelector(".source-mark");
  const demo = mark("Check alpha");
  const agent = mark("Check agent");
  check("a sidebar row has a mark", demo && agent);
  // A session in a git repository says so, and on which branch (GUI-044).
  const git = withText(".session-row", "Check alpha")?.querySelector(".git-mark");
  check("a row in a repository has the git mark", git && /^Git repository, on \S+/.test(git.getAttribute("aria-label")), git?.getAttribute("aria-label"));
  const title = withText(".session-row", "Check alpha").querySelector(".session-title");
  const gap = title.getBoundingClientRect().left - demo.getBoundingClientRect().right;
  const inner = document.createRange();
  inner.selectNodeContents(title);
  check("the title stands next to its mark", gap < 14 && inner.getBoundingClientRect().left - demo.getBoundingClientRect().right < 14, `${Math.round(inner.getBoundingClientRect().left - demo.getBoundingClientRect().right)} px apart`);
  const startColour = agent && getComputedStyle(agent).backgroundColor;
  check("a provider has its colour before the list of agents arrives", getComputedStyle(mark("Check agent")).backgroundColor !== getComputedStyle(document.documentElement).getPropertyValue("--source-none"));
  // The agents take a moment to list, each looked for through the login shell.
  await until("the agent's name", () => /^Provider: Claude Code/.test(mark("Check agent").getAttribute("aria-label")), 20000);
  check("the mark names the provider for a screen reader", /^Provider: Scripted demo/.test(demo.getAttribute("aria-label")), demo.getAttribute("aria-label"));
  check("two providers differ in letters", mark("Check alpha").textContent !== mark("Check agent").textContent, `${mark("Check alpha").textContent} and ${mark("Check agent").textContent}`);
  check("its colour stays once the list has arrived", getComputedStyle(mark("Check agent")).backgroundColor === startColour, `${startColour} became ${getComputedStyle(mark("Check agent")).backgroundColor}`);
  check("two providers differ in colour", getComputedStyle(mark("Check alpha")).backgroundColor !== getComputedStyle(mark("Check agent")).backgroundColor, getComputedStyle(mark("Check alpha")).backgroundColor);
  // The providers' list comes on its own; the colours settle once it has.
  const same = await until("one provider's colour", () => getComputedStyle(demo).backgroundColor === getComputedStyle(mark("Check long")).backgroundColor, 5000).catch(() => false);
  check("the sessions of one provider match", same, `${getComputedStyle(demo).backgroundColor} and ${getComputedStyle(mark("Check long")).backgroundColor}`);

  openSession("Check alpha");
  await until("the header", () => document.querySelector(".session-header .source-mark"));
  check("the top bar has the mark and the name", document.querySelector(".session-header .source-mark").textContent === demo.textContent && document.querySelector(".session-sub").textContent.startsWith("Scripted demo"), document.querySelector(".session-sub").textContent);

  // In a grid: each pane its own line, and nothing past a pane or row.
  document.querySelector('.layout-switch button[aria-label="2 panes"]').click();
  (await until("an offer", () => withText(".pane-offer", "Check agent"))).click();
  await until("two sessions in panes", () => all(".pane .session-header").length === 2);
  await sleep(200);
  const lines = all(".pane-split").map((p) => getComputedStyle(p, "::before").backgroundColor);
  check("each pane has its provider's line", lines.length === 2 && lines[0] !== lines[1] && lines.every((c) => c !== "rgba(0, 0, 0, 0)"), lines.join(" and "));
  const rowsFit = all(".session-row").every((r) => all("*", r).every((e) => e.getBoundingClientRect().right <= r.getBoundingClientRect().right + 1));
  check("nothing runs past a row", rowsFit);
  // The line stands clear of the active pane's outline, and the title clear of the line.
  const pane = document.querySelector(".pane-split.pane-active");
  const lineTop = parseFloat(getComputedStyle(pane, "::before").top);
  const lineBottom = pane.getBoundingClientRect().top + lineTop + parseFloat(getComputedStyle(pane, "::before").height);
  const titleTop = pane.querySelector(".session-titleline h2").getBoundingClientRect().top;
  check("the provider's line clears the pane's outline", lineTop >= 4, `${lineTop} px from the top`);
  check("the title clears the provider's line", titleTop - lineBottom >= 6, `${Math.round(titleTop - lineBottom)} px`);
  document.querySelector('.layout-switch button[aria-label="One pane"]').click();
  await sleep(200);

  // Choosing: at once, for every session on that provider.
  document.querySelector(".settings-link").click();
  (await until("the Appearance page", () => withText(".settings-nav button", "Appearance"))).click();
  const row = await until("the marks", () => all(".source-marks li").find((li) => li.textContent.includes("Claude Code")));
  const letters = row.querySelector("input");
  letters.focus();
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(letters, "Zz");
  letters.dispatchEvent(new Event("input", { bubbles: true }));
  await sleep(50);
  const free = all(".source-marks-slot", row).find((b) => b.getAttribute("aria-pressed") !== "true");
  const chosen = getComputedStyle(free).backgroundColor;
  free.click();
  await until("the row's mark", () => row.querySelector(".source-mark").textContent === "Zz", 3000);
  letters.blur();
  await sleep(800);
  document.querySelector(".settings-nav .settings-back").click();
  await until("the sidebar row", () => mark("Check agent"));
  await sleep(200);
  check("chosen letters apply", mark("Check agent").textContent === "Zz", mark("Check agent").textContent);
  check("a chosen colour applies", getComputedStyle(mark("Check agent")).backgroundColor === chosen, getComputedStyle(mark("Check agent")).backgroundColor);
});

// A profile on an agent with its own sign-in hides the provider, and says how to get it.
await step("a pane shows how full its context is", async () => {
  const meter = () => activePane().querySelector(".session-titleline .context-meter");
  const rowMeter = (title) => withText(".session-row", title)?.querySelector(".context-meter");
  const compactButton = () => named(".session-actions button", "Compact context", activePane());
  const open = async (title) => {
    openSession(title);
    await until(`${title} to open`, () => activePane().querySelector(".session-titleline h2")?.textContent === title);
  };
  await open("Check long");
  const low = await until("the meter", meter);
  check("a session's context shows on its pane", /^(<1|\d+) %$/.test(low.textContent) && !low.classList.contains("context-high"), low.textContent);
  check("the meter explains itself", /tokens/.test(low.title), low.title);
  check("an ordinary context offers no Compact", !compactButton());
  await open("Check agent");
  await sleep(300);
  check("no figure, no meter", !meter(), meter()?.textContent);
  // The real engine, with a prompt that fills the demo's window and a call that stays last.
  await open("Check full");
  const box = await until("the message box", () => activePane().querySelector(".composer textarea"));
  box.focus();
  type(box, `[no steps] ${"the quick brown fox jumps over a lazy dog\n".repeat(8100)}`);
  await sleep(100);
  (await until("Send", () => { const b = named(".composer-actions button.primary", "Send", activePane()); return b && !b.disabled && b; })).click();
  await until("the meter to rise", () => meter()?.classList.contains("context-high"), 30000);
  check("a full context stands out", /^8\d %$/.test(meter().textContent), meter().textContent);
  check("a full context shows in the sidebar", rowMeter("Check full")?.classList.contains("context-high"), rowMeter("Check full")?.textContent);
  check("a quiet context stays out of the sidebar", !rowMeter("Check long"));
  const compact = await until("Compact to be enabled", () => { const b = compactButton(); return b && !b.disabled && b; }, 30000);
  compact.click();
  await until("the context to fall", () => meter() && !meter().classList.contains("context-high"), 30000);
  check("Compact compacts", withText(".compaction summary", "Context compacted", activePane()));
  check("Compact goes once the context has room", !compactButton());
  check("the sidebar's figure goes with it", !rowMeter("Check full"));
  // A limit the user sets reaches a running session's next call, and can be taken back.
  const panelToggle = named(".session-actions button", "Panel", activePane());
  const panelWasOpen = panelToggle.getAttribute("aria-pressed") === "true";
  if (!panelWasOpen) panelToggle.click();
  (await until("the Info tab", () => withText(".panel-tabs [role=tab]", "Info"))).click();
  const field = await until("the limit's field", () => document.querySelector("aside.panel .context-limit input"));
  // In the Mac's own number format.
  const detected = `${(100000).toLocaleString()} (detected)`;
  // The model's limit is looked up when the tab opens.
  await until("the detected limit", () => field.placeholder === detected, 8000).catch(() => {});
  check("a detected limit shows as such", field.placeholder === detected, field.placeholder);
  const setLimit = async (value) => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(field, value);
    field.dispatchEvent(new Event("input", { bubbles: true }));
    (await until("Save", () => withText("aside.panel .context-limit button", "Save"))).click();
    await until("the limit to save", () => !withText("aside.panel .context-limit button", "Save"));
  };
  await setLimit("1k");
  check("a limit is kept as typed", field.value === "1000", field.value);
  const again = activePane().querySelector(".composer textarea");
  again.focus();
  type(again, "[no steps] again");
  await sleep(100);
  (await until("Send", () => { const b = named(".composer-actions button.primary", "Send", activePane()); return b && !b.disabled && b; })).click();
  await until("the share of the new limit", () => /^\d+ %$/.test(meter()?.textContent ?? "") && parseInt(meter().textContent) >= 5, 15000);
  check("a running session takes a new limit", true);
  await setLimit("");
  check("clearing a limit goes back to what is detected", field.placeholder === detected, field.placeholder);
  withText(".panel-tabs [role=tab]", "Approvals").click();
  if (!panelWasOpen) panelToggle.click();
});

await step("a provider can be reached from any profile", async () => {
  named(".sidebar header button", "New session").click();
  const profile = await until("the new-session form", () => withText("label", "Profile")?.querySelector("select"));
  const external = [...profile.options].find((o) => o.value === "claude-code");
  if (!external) throw new Error("no Claude Code profile to start from");
  Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(profile, external.value);
  profile.dispatchEvent(new Event("change", { bubbles: true }));
  await sleep(200);
  const provider = () => [...document.querySelectorAll("label")].find((l) => l.firstChild?.textContent?.trim() === "Provider")?.querySelector("select");
  check("an agent's own profile hides the provider", !provider());
  const offer = await until("the offer", () => withText(".provider-offer button", "Use a provider"), 3000);
  offer.click();
  await until("the provider field", provider, 3000);
  check("the offer brings the provider field", [...provider().options].some((o) => o.textContent.includes("Scripted demo")));
  check("the offer goes once used", !document.querySelector(".provider-offer"));
});

await step("a session's right-click menu", async () => {
  const mouse = (...a) => window.webkit.messageHandlers.mouse.postMessage(a);
  const rightClick = async (title) => {
    const b = withText(".session-row", title).getBoundingClientRect();
    await mouse(b.left + 40, b.top + b.height / 2, -1);
    return until("the menu", () => document.querySelector(".context-menu"), 4000);
  };
  const item = (text) => withText(".context-menu button", text);
  const before = location.href;
  let menu = await rightClick("Check menu");
  check("a real right click opens the menu", menu && item("Rename") && item("Delete"));
  check("the menu opens inside the window", menu.getBoundingClientRect().right <= innerWidth && menu.getBoundingClientRect().bottom <= innerHeight);
  check("it does not open the session", !document.querySelector(".session-header h2") || !document.querySelector(".session-header h2").textContent.includes("Check menu"));
  menu.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  await until("the menu to close", () => !document.querySelector(".context-menu"), 2000);
  check("Escape closes the menu", true);

  // Renaming, in the row.
  await rightClick("Check menu");
  item("Rename").click();
  const name = await until("the name field", () => document.querySelector(".session-rename input"));
  const setName = (v) => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(name, v);
    name.dispatchEvent(new Event("input", { bubbles: true }));
  };
  setName("Check menu renamed");
  name.form.requestSubmit();
  await until("the new name", () => withText(".session-row", "Check menu renamed"), 5000);
  check("renaming in the row", true);

  await rightClick("Check menu renamed");
  item("Rename").click();
  const again = await until("the name field", () => document.querySelector(".session-rename input"));
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(again, "Should not stick");
  again.dispatchEvent(new Event("input", { bubbles: true }));
  await sleep(50);
  again.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  await until("the row back", () => !document.querySelector(".session-rename"), 2000);
  await sleep(500);
  check("Escape leaves the name as it was", withText(".session-row", "Check menu renamed") && !withText(".session-row", "Should not stick"));

  // Deleting asks first. A served build has no native dialogs, so this answers that call.
  let answer = "Cancel";
  let asked = 0;
  const real = window.fetch;
  window.fetch = (u, o) => {
    const text = decodeURIComponent(`${u}${o?.body ?? ""}`);
    if (text.includes("removed from disk")) {
      asked++;
      return Promise.resolve(new Response(JSON.stringify(answer), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    return real(u, o);
  };
  await rightClick("Check menu renamed");
  item("Delete").click();
  await sleep(600);
  check("deleting asks first", asked === 1, `asked ${asked} times`);
  check("declining keeps the session", withText(".session-row", "Check menu renamed"));
  answer = "OK";
  await rightClick("Check menu renamed");
  item("Delete").click();
  await until("the session to go", () => !withText(".session-row", "Check menu renamed"), 5000);
  check("deleting removes the session", true);
  window.fetch = real;
});

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

// No demo session has subagents, so this goes by the rule, which a broader one has hidden.
await step("subagents are indented under their parent", async () => {
  const list = document.createElement("ul");
  list.className = "subagents";
  const row = document.querySelector(".session-row").closest("li");
  row.appendChild(list);
  const style = getComputedStyle(list);
  check("subagents are indented under their parent", parseInt(style.marginLeft) + parseInt(style.paddingLeft) >= 20, `${style.marginLeft} and ${style.paddingLeft}`);
  list.remove();
  const outline = getComputedStyle(document.querySelector(".session-row")).borderTopWidth;
  check("a chat has an outline", outline === "1px", outline);
});

await step("a draft stays until sent or deleted", async () => {
  const box = () => activePane().querySelector(".composer textarea");
  type(box(), "a draft to keep");
  await sleep(100);
  document.querySelector(".settings-link").click();
  await until("Settings", () => document.querySelector(".settings-nav"));
  check("the composer is gone in Settings", !document.querySelector(".composer textarea"));
  await sleep(600);
  document.querySelector(".settings-nav .settings-back").click();
  await until("the session again", () => activePane().querySelector(".composer textarea"));
  check("a draft survives going to Settings", box().value === "a draft to keep", `it reads "${box().value}"`);
  const sid = Object.keys(localStorage).find((k) => k.startsWith("freeloop.draft."));
  check("the draft is stored for a restart", sid && localStorage.getItem(sid) === "a draft to keep", sid);
  openSession("Check beta");
  await until("the other session", () => document.querySelector(".session-header h2")?.textContent.includes("Check beta"));
  await sleep(200);
  check("a draft belongs to its session", box().value === "", `it reads "${box().value}"`);
  openSession("Check alpha");
  await until("the session again", () => document.querySelector(".session-header h2")?.textContent.includes("Check alpha"));
  await sleep(200);
  check("it is there on coming back", box().value === "a draft to keep", `it reads "${box().value}"`);
  type(box(), "");
  await sleep(100);
  check("emptying the box deletes the draft", !Object.keys(localStorage).some((k) => k.startsWith("freeloop.draft.")));
  type(box(), "this one gets sent");
  await until("the draft to be stored", () => Object.keys(localStorage).some((k) => k.startsWith("freeloop.draft.")), 5000);
  check("typing stores it again", true);
  type(box(), "");
  await sleep(100);
});

await step("the file viewer shows Markdown formatted", async () => {
  const panelButton = named(".session-actions button", "Panel");
  if (panelButton.getAttribute("aria-pressed") !== "true") panelButton.click();
  (await until("the Files tab", () => withText(".panel-tabs [role=tab]", "Files"))).click();
  (await until("guide.md", () => withText(".file-tree button", "guide.md"))).click();
  const view = await until("the formatted file", () => document.querySelector(".file-markdown"));
  check("Markdown shows formatted", view.querySelector("h1, h2, h3") && view.querySelector("strong"), view.innerHTML.slice(0, 80));
  await sleep(200);
  check("markup in a file does not run", !view.querySelector("img") && !view.querySelector("[onerror]") && !window.pwned && !view.querySelector('a[href^="javascript"]'), view.innerHTML.slice(0, 200));
  withText(".file-view-switch button", "Source").click();
  await until("the source", () => document.querySelector(".file-view .file-text"));
  check("the source shows the raw text", document.querySelector(".file-text").textContent.includes("# A heading") && !document.querySelector(".file-markdown"));
  (await until("paper.pdf", () => withText(".file-tree button", "paper.pdf"))).click();
  // The viewer holds what a PDF can do: see pdfview.ts.
  const outer = await until("the PDF's viewer", () => document.querySelector("iframe.file-pdf"));
  const inner = await until("the PDF's frame", () => {
    const f = outer.contentDocument?.getElementById("pdf");
    return f?.src?.startsWith("blob:") && f;
  });
  await until("the PDF to open", () => inner.contentDocument?.contentType === "application/pdf", 8000);
  check("a PDF opens in the viewer", true);
  check("the viewer carries its policy", outer.contentDocument.querySelector('meta[http-equiv="Content-Security-Policy"]')?.content.includes("frame-src blob:"));
  check("the fallback to Preview stays", withText(".file-view button", "Open in Preview"));
  const before = location.href;
  const box = inner.getBoundingClientRect();
  const at = outer.getBoundingClientRect();
  await sleep(800);
  await window.webkit.messageHandlers.mouse.postMessage([at.left + box.left + box.width / 2, at.top + box.top + box.height / 2, 1]);
  await sleep(1500);
  check("a click on a PDF's link leaves the app where it is", location.href === before && !!document.querySelector(".sidebar"), location.href);
  let stayed;
  try {
    stayed = outer.contentDocument.getElementById("pdf").contentWindow.location.href;
  } catch {
    stayed = "gone to another origin";
  }
  check("a click on a PDF's link leaves the PDF where it is", stayed.startsWith("blob:"), stayed);
  check("a script in a PDF did not run", !window.pwned);
  withText(".file-tree button", "fake.pdf").click();
  await until("the refusal", () => document.querySelector(".file-view")?.textContent.includes("not one"), 5000);
  check("a file only named as a PDF is refused", !document.querySelector("iframe.file-pdf") && !window.pwned);
  (withText(".file-tree button", "guide.md")).click();
  await until("Markdown again, formatted", () => document.querySelector(".file-markdown"));
  check("a Markdown file opens formatted again", true);
  withText(".panel-tabs [role=tab]", "Approvals").click();
  await sleep(100);
});

await step("a turn runs to its end, with approvals", async () => {
  const line = await turn("Check alpha");
  check("a turn runs to its end, with approvals", true);
  check("the sidebar names what a waiting session asks", line.startsWith("Needs you: "), `it said "${line}"`);
});

await step("the view follows a running agent", async () => {
  // Enough to overflow, and cards collapse along the way.
  await turn("Check alpha");
  check("sending deletes the draft", activePane().querySelector(".composer textarea").value === "" && !Object.keys(localStorage).some((k) => k.startsWith("freeloop.draft.")));
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

await step("a click clears a selection", async () => {
  const mouse = (...a) => window.webkit.messageHandlers.mouse.postMessage(a);
  const selected = () => getSelection().toString().length;
  const selectAll = async () => {
    document.activeElement?.blur?.();
    document.execCommand("selectAll");
    await sleep(100);
    if (!selected()) throw new Error("nothing got selected");
  };
  const at = (el, x = 0.5, y = 0.5) => {
    const b = el.getBoundingClientRect();
    return [b.left + b.width * x, b.top + b.height * y];
  };
  const clicked = async (name, el, x, y) => {
    await selectAll();
    // DIAGNOSIS (CI, macOS 15): what WebKit sends, and the selection at each step.
    const seen = [];
    const log = (e) => seen.push(`${e.type}(d${e.detail},${e.target?.className?.toString?.().slice(0, 20) ?? e.target?.nodeName}):${selected()}`);
    const kinds = ["mousedown", "mouseup", "click", "selectstart", "selectionchange"];
    for (const k of kinds) document.addEventListener(k, log, true);
    await mouse(...at(el, x, y), 1);
    requestAnimationFrame(() => seen.push(`frame:${selected()}`));
    await sleep(150);
    for (const k of kinds) document.removeEventListener(k, log, true);
    check(`a click clears a selection: ${name}`, selected() === 0, `${selected()} characters stayed selected; ${seen.join(" ")}`);
  };
  const msg = all(".msg-agent", activePane()).at(-1);
  // DIAGNOSIS (CI, macOS 15): where the message and the view are before the click.
  {
    const c = conversation();
    const r = msg.getBoundingClientRect();
    const [x, y] = at(msg);
    results.push({ name: "DIAGNOSIS before the text click", ok: true, detail: `window ${innerWidth}x${innerHeight}; conversation top ${Math.round(c.scrollTop)} of ${c.scrollHeight}, shows ${c.clientHeight}; message ${Math.round(r.top)}..${Math.round(r.bottom)}, click at ${Math.round(x)},${Math.round(y)}; ${all(".msg-agent", activePane()).length} agent messages` });
    await window.webkit.messageHandlers.snap?.postMessage("before the text click");
  }
  await clicked("text", msg);
  await clicked("the conversation's margin", conversation(), 0.01, 0.5);
  await clicked("the header's title", document.querySelector(".session-header h2"));
  const nav = document.querySelector(".sidebar nav");
  await clicked("the sidebar's empty space", nav, 0.5, 0.98);
  await clicked("the footer's empty space", document.querySelector(".footer-links"), 0.98, 0.5);
  // A button still does its work, and leaves no selection behind.
  const panel = named(".session-actions button", "Panel");
  const pressed = panel.getAttribute("aria-pressed");
  await clicked("a button", panel);
  check("the button still worked", panel.getAttribute("aria-pressed") !== pressed);
  await selectAll();
  await mouse(...at(panel), 1);
  await sleep(200);
  // What selecting by hand does must keep working.
  await mouse(...at(msg, 0.1), 2);
  await sleep(150);
  check("a double click selects a word", selected() > 0 && selected() < 40, `${selected()} characters`);
  await mouse(...at(document.querySelector(".session-header h2")), 1);
  await sleep(150);
  const b = msg.getBoundingClientRect();
  await mouse(b.left + 10, b.top + 10, 1, b.left + 120, b.top + 10);
  await sleep(150);
  check("dragging selects text, and it stays", selected() > 0, "nothing selected");
  await mouse(...at(msg, 0.1), 3);
  await sleep(150);
  check("a triple click selects a line", selected() > 0, "nothing selected");
  await mouse(...at(document.querySelector(".session-header h2")), 1);
  await sleep(150);
});

await step("the panel's tabs clear its close button", async () => {
  const panel = named(".session-actions button", "Panel");
  if (panel.getAttribute("aria-pressed") !== "true") panel.click();
  const close = await until("the panel", () => document.querySelector(".panel-close"));
  const last = all(".panel-tablist [role=tab]").at(-1);
  const list = document.querySelector(".panel-tablist");
  check("the panel's tabs clear its close button", last.getBoundingClientRect().right <= close.getBoundingClientRect().left + 1 && list.scrollWidth <= list.clientWidth, `the last tab ends at ${Math.round(last.getBoundingClientRect().right)}, the button starts at ${Math.round(close.getBoundingClientRect().left)}`);
});

await step("the year at a glance", async () => {
  named(".footer-links button", "Usage").click();
  const box = await until("the overview", () => document.querySelector(".usage-overview"), 8000);
  const cells = await until("the calendar", () => { const c = all(".usage-calendar-day", box); return c.length > 300 && c; }, 8000);
  check("the calendar has a square per day", cells.length >= 365 && cells.length <= 371, `${cells.length} squares`);
  check("the overview shows five figures", all(".usage-overview-tiles strong", box).length === 5);
  const used = cells.filter((c) => !c.getAttribute("aria-label").endsWith("no use"));
  check("the turns above show as today's use", used.length >= 1 && all(".usage-calendar-model", box).length >= 1, `${used.length} days with use, ${all(".usage-calendar-model", box).length} models`);
  check("a used day has its colour", used.length === 0 || getComputedStyle(used[0]).backgroundColor !== getComputedStyle(cells.find((c) => c.getAttribute("aria-label").endsWith("no use"))).backgroundColor);
  const first = box.querySelector('.usage-calendar-day[tabindex="0"]');
  check("the newest day is the one in the Tab order", first === cells.filter((c) => c.dataset.day.startsWith(`${Math.max(...cells.map((x) => parseInt(x.dataset.day)))}-`)).at(-1));
  first.focus();
  const before = document.activeElement;
  document.activeElement.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowLeft", bubbles: true }));
  await sleep(100);
  check("the arrow keys walk the days", document.activeElement !== before && document.activeElement.classList.contains("usage-calendar-day"));
  check("pointing says what a day holds", box.querySelector(".usage-calendar-readout").textContent.includes(":"), box.querySelector(".usage-calendar-readout").textContent);
  check("the calendar fits the page", box.getBoundingClientRect().right <= document.querySelector(".usage-page").getBoundingClientRect().right + 1);
  openSession("Check alpha");
  await until("the conversation", () => document.querySelector(".conversation"));
});

await step("the queue takes a task and holds it while paused", async () => {
  named(".footer-links button", "Board").click();
  (await until("the board's link to the queue", () => document.querySelector(".board-queue"), 8000)).click();
  const page = await until("the queue page", () => [...document.querySelectorAll(".page h2")].find((h) => h.textContent === "Queue")?.closest(".page"), 8000);
  if (withText("button", "Pause", page)) withText("button", "Pause", page).click();
  await until("paused", () => page.querySelector('[role="status"]')?.textContent.startsWith("Paused"), 5000);
  const box = page.querySelector('textarea[aria-label="Task"]');
  Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value").set.call(box, "Write a haiku about queues");
  box.dispatchEvent(new Event("input", { bubbles: true }));
  await sleep(50);
  withText("button", "Add to queue", page).click();
  const row = await until("the task listed", () => [...page.querySelectorAll(".queue-table tbody tr")].find((r) => r.textContent.includes("Write a haiku")), 5000);
  check("a paused queue keeps the task waiting", row.textContent.includes("Waiting"), row.textContent);
  check("the summary counts it", page.querySelector('[role="status"]').textContent.includes("1 waiting"), page.querySelector('[role="status"]').textContent);
  row.querySelector('button[aria-label^="Remove"]').click();
  await until("the task removed", () => !page.textContent.includes("Write a haiku"), 5000);
  openSession("Check alpha");
  await until("the conversation", () => document.querySelector(".conversation"));
});

await step("the side parts are resizable", async () => {
  const mouse = (...a) => window.webkit.messageHandlers.mouse.postMessage(a);
  const centre = (el) => { const b = el.getBoundingClientRect(); return [b.left + b.width / 2, b.top + b.height / 2]; };
  const sidebar = document.querySelector(".sidebar");
  const was = sidebar.getBoundingClientRect().width;
  let [x, y] = centre(document.querySelector(".splitter-sidebar"));
  await mouse(x, y, 1, x + 100, y);
  await sleep(200);
  check("dragging widens the sidebar", Math.abs(sidebar.getBoundingClientRect().width - was - 100) <= 8, `${Math.round(was)} became ${Math.round(sidebar.getBoundingClientRect().width)} px`);
  check("the sidebar's width is remembered", Number(localStorage.getItem("freeloop.sidebarWidth")) > was);
  [x, y] = centre(document.querySelector(".splitter-sidebar"));
  await mouse(x, y, 1, 20, y);
  await sleep(200);
  check("the sidebar stops at its least", Math.round(sidebar.getBoundingClientRect().width) === 180, `${Math.round(sidebar.getBoundingClientRect().width)} px`);
  [x, y] = centre(document.querySelector(".splitter-sidebar"));
  await mouse(x, y, 2);
  await sleep(200);
  check("a double click puts the sidebar back", Math.round(sidebar.getBoundingClientRect().width) === 264, `${Math.round(sidebar.getBoundingClientRect().width)} px`);

  const panelButton = named(".session-actions button", "Panel");
  if (panelButton.getAttribute("aria-pressed") !== "true") panelButton.click();
  const panel = await until("the panel", () => document.querySelector("aside.panel"));
  const usual = panel.getBoundingClientRect().width;
  [x, y] = centre(document.querySelector(".splitter-panel"));
  await mouse(x, y, 1, x - 120, y);
  await sleep(200);
  check("dragging widens the panel", Math.abs(panel.getBoundingClientRect().width - usual - 120) <= 8, `${Math.round(usual)} became ${Math.round(panel.getBoundingClientRect().width)} px`);
  const talk = activePane().querySelector(".conversation");
  [x, y] = centre(document.querySelector(".splitter-panel"));
  await mouse(x, y, 1, 10, y);
  await sleep(200);
  check("a wide panel leaves the conversation its room", talk.getBoundingClientRect().width >= 279, `${Math.round(talk.getBoundingClientRect().width)} px`);
  const line = document.querySelector(".splitter-panel");
  line.focus();
  const before = panel.getBoundingClientRect().width;
  line.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowRight", bubbles: true }));
  await sleep(100);
  check("an arrow key moves a divider", Math.abs(panel.getBoundingClientRect().width - before + 16) <= 1, `${before} became ${panel.getBoundingClientRect().width} px`);
  [x, y] = centre(document.querySelector(".splitter-panel"));
  await mouse(x, y, 2);
  await sleep(200);
  check("a double click puts the panel back", Math.round(panel.getBoundingClientRect().width) === Math.round(usual), `${Math.round(panel.getBoundingClientRect().width)} against ${Math.round(usual)} px`);
});

await step("a finished session says so on the board", async () => {
  named(".footer-links button", "Board").click();
  const card = await until("the board's card", () => withText(".board-card", "Check alpha"));
  const line = card.querySelector(".session-activity")?.textContent ?? "";
  check("a finished session says so on the board", line.startsWith("Finished"), `it said "${line}"`);
  openSession("Check alpha");
  await until("the conversation", () => document.querySelector(".conversation"));
});

await step("the More menu closes like a menu", async () => {
  const mouse = (...a) => window.webkit.messageHandlers.mouse.postMessage(a);
  const at = (el) => { const b = el.getBoundingClientRect(); return [b.left + b.width / 2, b.top + b.height / 2]; };
  const menu = activePane().querySelector(".export-menu");
  await mouse(...at(menu.querySelector("summary")), 1);
  await until("the menu to open", () => menu.open, 4000);
  const away = activePane().querySelector(".conversation");
  const b = away.getBoundingClientRect();
  await mouse(b.left + b.width / 2, b.top + 40, 1);
  check("a click elsewhere closes the More menu", await until("the menu to close", () => !menu.open, 4000).catch(() => false));
  await mouse(...at(menu.querySelector("summary")), 1);
  await until("the menu to open", () => menu.open, 4000);
  menu.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  await sleep(50);
  check("Escape closes the More menu", !menu.open);
});

await step("the app's settings are seven short pages", async () => {
  // Each page holds its own sections, and the settings open on the page last shown.
  document.querySelector(".settings-link").click();
  await until("Settings", () => document.querySelector(".settings-nav"));
  const where = { Appearance: "Theme and text size", Keyboard: "", Sessions: "Notifications", Costs: "Budgets", Permissions: "Bypass mode", Network: "Web search", Data: "Backup" };
  const only = ["Theme and text size", "Notifications", "Budgets", "Bypass mode", "Web search", "Backup"];
  for (const [page, section] of Object.entries(where)) {
    withText(".settings-nav button", page).click();
    const h2 = await until(`the ${page} page`, () => { const h = document.querySelector(".settings-page h2"); return h?.textContent === page && h; });
    const here = all(".settings-page h3").map((h) => h.textContent);
    check(`${page} holds its own sections`, (!section || here.includes(section)) && only.filter((x) => x !== section).every((x) => !here.includes(x)), here.join(", "));
    check(`${page} is not a long page`, document.querySelector(".settings-page").scrollHeight < 2600, `${document.querySelector(".settings-page").scrollHeight} px`);
  }
  document.querySelector(".settings-nav .settings-back").click();
  await sleep(100);
  document.querySelector(".settings-link").click();
  const back = await until("the page last shown", () => document.querySelector(".settings-page h2"));
  check("the settings open on the page last shown", back.textContent === "Data", back.textContent);
  document.querySelector(".settings-nav .settings-back").click();
  await sleep(100);
});

await step("a budget shows on the usage page", async () => {
  // PRV-017: set beside the prices, shown with what this period spent.
  const setInput = (el, value) => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(el, value);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  };
  const general = async () => {
    document.querySelector(".settings-link").click();
    (await until("the Costs page", () => withText(".settings-nav button", "Costs"))).click();
  };
  const save = async () => {
    (await until("Save", () => document.querySelector(".settings-save button.primary"))).click();
    await until("saved", () => !document.querySelector(".settings-save"), 8000);
  };
  await general();
  (await until("Add a budget", () => withText("button", "Add a budget"))).click();
  setInput(await until("the amount", () => document.querySelector("input[aria-label='Amount in US dollars']")), "0.01");
  await sleep(50);
  await save();
  document.querySelector(".settings-nav .settings-back").click();
  named(".footer-links button", "Usage").click();
  const card = await until("the budget", () => document.querySelector(".budgets"), 8000);
  check("a budget shows on the usage page", card.textContent.includes("Daily budget, $0.01") && card.textContent.includes("spent"), card.textContent.slice(0, 160));
  await general();
  (await until("Remove", () => document.querySelector(".budget-row button.danger"))).click();
  await save();
  document.querySelector(".settings-nav .settings-back").click();
  openSession("Check alpha");
  await until("the conversation", () => document.querySelector(".conversation"));
});

await step("the policy page says what a call would meet", async () => {
  // SEC-018: the broker's own judgment of a call not made, with the rule that decided.
  const set = (el, value) => {
    const proto = el instanceof HTMLSelectElement ? HTMLSelectElement.prototype : HTMLInputElement.prototype;
    Object.getOwnPropertyDescriptor(proto, "value").set.call(el, value);
    el.dispatchEvent(new Event(el instanceof HTMLSelectElement ? "change" : "input", { bubbles: true }));
  };
  document.querySelector(".settings-link").click();
  (await until("the Policy page", () => withText(".settings-nav button", "Policy"))).click();
  const form = await until("the check", () => document.querySelector(".policy-check"));
  set(document.querySelector("select[aria-label=Effect]"), "deny");
  set(document.querySelector("select[aria-label=Tool]"), "shell");
  await sleep(50);
  set(document.querySelector("input[aria-label=Command]"), "git push *");
  await sleep(50);
  withText("button", "Add rule").click();
  await until("the rule", () => withText(".provider-list li", "git push *"));
  const ask = async (what) => {
    set(form.querySelector("select[aria-label=Action]"), "shell");
    await sleep(50);
    set(form.querySelector("input[aria-label='Command to check']"), what);
    await sleep(50);
    form.querySelector("button[type=submit]").click();
    return until(`the verdict on ${what}`, () => { const v = form.querySelector(".policy-verdict"); return v && v.textContent.includes(what.split(" ")[0] === "git" ? "git push" : "asked") && v; }, 8000);
  };
  const denied = await ask("git push origin main");
  check("a deny rule shows as what decides", denied.textContent.startsWith("Denied.") && denied.textContent.includes("shell: git push *"), denied.textContent.slice(0, 160));
  const asked = await ask("make deploy");
  check("a call no rule decides asks", asked.textContent.startsWith("You would be asked."), asked.textContent.slice(0, 160));
  check("the page shows what else decides", !!withText("h3", "What else decides") && !!withText("summary", "Protected paths"));
  withText(".provider-list li", "git push *").querySelector("button.danger").click();
  await until("the rule gone", () => !withText(".provider-list li", "git push *"));
  document.querySelector(".settings-nav .settings-back").click();
  await until("the conversation", () => document.querySelector(".conversation"));
});

await step("every control has a name", async () => {
  // GUI-041: a control drawn as an icon alone is named in aria-label and explained in a tooltip.
  const unnamed = [];
  const untitled = [];
  for (const e of all("button, a[href], summary, [role=tab], [role=menuitem]")) {
    if (!e.getClientRects().length || e.closest("[aria-hidden=true]")) continue;
    const text = e.textContent.trim();
    const name = e.getAttribute("aria-label") || (e.getAttribute("aria-labelledby") && "labelled") || text;
    const what = e.outerHTML.slice(0, 120);
    if (!name) unnamed.push(what);
    else if (!text && !e.title) untitled.push(what);
  }
  check("every control has an accessible name", unnamed.length === 0, unnamed.join(" | "));
  check("every icon-only control has a tooltip", untitled.length === 0, untitled.join(" | "));
  check("the sidebar does not show what a session runs", all(".session-row .session-activity").length === 0);
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
  // The panes' divider moves their columns, and a double click puts it back.
  {
    const mouse = (...a) => window.webkit.messageHandlers.mouse.postMessage(a);
    const line = document.querySelector(".splitter-panes");
    const [left] = all(".pane").sort((a, b) => a.getBoundingClientRect().left - b.getBoundingClientRect().left);
    const middle = () => { const b = line.getBoundingClientRect(); return [b.left + b.width / 2, b.top + b.height / 2]; };
    const was = left.getBoundingClientRect().width;
    const [x, y] = middle();
    await mouse(x, y, 1, x + 120, y);
    await sleep(200);
    const moved = left.getBoundingClientRect().width;
    check("dragging the panes' divider moves them", Math.abs(moved - was - 120) <= 12, `${Math.round(was)} became ${Math.round(moved)} px`);
    const nothingOver = all(".pane").every((p) => all("*", p).every((e) => e.closest(".export-options") || e.getBoundingClientRect().width === 0 || e.getBoundingClientRect().right <= p.getBoundingClientRect().right + 1));
    check("nothing runs past a resized pane", nothingOver);
    const [mx, my] = middle();
    await mouse(mx, my, 2);
    await sleep(200);
    check("a double click evens the panes", Math.abs(left.getBoundingClientRect().width - was) <= 2, `${Math.round(left.getBoundingClientRect().width)} against ${Math.round(was)} px`);
  }

  // A long prompt in a short pane scrolls in its box and leaves Send in the pane.
  document.querySelector('.layout-switch button[aria-label="4 panes"]').click();
  await sleep(300);
  const short = await until("a session in the grid", () => all(".pane").find((p) => p.querySelector(".composer textarea")));
  const prompt = short.querySelector(".composer textarea");
  prompt.focus();
  await sleep(200);
  type(prompt, "a long line of a prompt\n".repeat(200));
  await sleep(300);
  const paneBox = short.getBoundingClientRect();
  const send = named(".composer-actions button.primary", "Send", short);
  const talk = short.querySelector(".conversation");
  check("a long prompt leaves Send in the pane", send && send.getBoundingClientRect().bottom <= paneBox.bottom, `Send ends at ${Math.round(send?.getBoundingClientRect().bottom)}, the pane at ${Math.round(paneBox.bottom)}`);
  check("a long prompt leaves room for the conversation", talk.clientHeight >= 60, `${talk.clientHeight} px left`);
  type(prompt, "");
  await sleep(200);
  check("the box shrinks back with the prompt gone", prompt.getBoundingClientRect().height < 80, `${Math.round(prompt.getBoundingClientRect().height)} px`);
  document.querySelector('.layout-switch button[aria-label="2 panes"]').click();
  await sleep(300);

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

await step("a session's header has the git mark", async () => {
  openSession("Check alpha");
  await until("Check alpha", () => activePane().querySelector(".session-titleline h2")?.textContent === "Check alpha");
  const mark = await until("the git mark", () => activePane().querySelector(".session-titleline .git-mark"));
  const title = activePane().querySelector(".session-titleline h2").getBoundingClientRect();
  const at = mark.getBoundingClientRect();
  check("the header has the git mark, after the title", at.left >= title.right - 1 && at.width > 0 && at.width <= 16, `${Math.round(at.left)} after ${Math.round(title.right)}, ${Math.round(at.width)} px wide`);
});

await step("a plan reads as a document and is reviewed in the panel", async () => {
  openSession("Check plan");
  await until("Check plan", () => activePane().querySelector(".session-titleline h2")?.textContent === "Check plan");
  const pane = activePane();
  const send = async (text) => {
    const box = await until("the message box", () => pane.querySelector(".composer textarea"));
    box.focus();
    type(box, text);
    await sleep(80);
    (await until("Send", () => { const b = pane.querySelector(".composer-send"); return b && !b.disabled && b; })).click();
  };
  const setValue = (el, value) => {
    const proto = el instanceof HTMLSelectElement ? HTMLSelectElement.prototype : HTMLTextAreaElement.prototype;
    Object.getOwnPropertyDescriptor(proto, "value").set.call(el, value);
    el.dispatchEvent(new Event(el instanceof HTMLSelectElement ? "change" : "input", { bubbles: true }));
  };
  await send("[plan] tidy the code");
  const card = await until("the plan", () => pane.querySelector(".plan-card"), 20000);
  check("a plan is formatted", card.querySelector("h1, h2, h3, h4")?.textContent === "The plan" && card.querySelectorAll("li").length === 2, card.innerHTML.slice(0, 120));
  const tool = card.closest("details");
  check("a plan is shown once, not as its input", !tool.querySelector(".tool-input") && !tool.textContent.includes("# The plan"));
  const approval = await until("the plan's decision", () => pane.querySelector(".plan-approval"));
  const menu = [...approval.querySelectorAll("select option")].map((o) => o.textContent);
  check("a plan's options say what follows", menu.join("|") === "Ask before edits|Accept edits", menu.join("|"));
  check("a plan is approved or sent back", withText("button", "Approve", approval) && withText("button", "Keep planning", approval));

  card.querySelector(".plan-review-link").click();
  const panel = await until("the Plan tab", () => document.querySelector("aside.panel .plan-panel"));
  check("the card opens the Plan tab", withText(".panel-tabs [role=tab]", "Plan")?.getAttribute("aria-selected") === "true");
  const list = [...panel.querySelectorAll(".plan-block")].find((b) => b.querySelector("li"));
  list.querySelector(".plan-comment-add").click();
  setValue(await until("the comment box", () => list.querySelector(".plan-comment")), "Add a test for the parser.");
  const back = await until("Keep planning", () => withText("button", "Keep planning (1 comment)", panel));
  back.click();
  const sentBack = await until("the plan to go back", () => withText("p", "Sent back with:", document.querySelector("aside.panel .plan-panel")), 15000);
  check("a comment goes back quoting its line", sentBack.textContent.includes("> Read the code") && sentBack.textContent.includes("Add a test for the parser."), sentBack.textContent);

  await send("[plan] again");
  const version = await until("the second version", () => {
    const sel = document.querySelector("aside.panel .plan-panel select[aria-label=Version]");
    return sel && sel.options.length === 2 && sel;
  }, 20000);
  check("the latest version shows", version.selectedIndex === 1, version.value);
  const toggle = withText("aside.panel .plan-changes-toggle", "Changes since version 1").querySelector("input");
  toggle.click();
  const diff = await until("the changes", () => document.querySelector("aside.panel .plan-diff"));
  check("the changes since the last version show", diff.textContent.includes("+ 3. Change the code"), diff.textContent.slice(0, 200));
  toggle.click();
  const decision = await until("the decision", () => document.querySelector("aside.panel .plan-decision"));
  setValue(decision.querySelector("select"), [...decision.querySelectorAll("select option")].find((o) => o.textContent === "Accept edits").value);
  withText("button", "Approve", decision).click();
  await until("the plan approved", () => document.querySelector("aside.panel .plan-panel select[aria-label=Version]")?.selectedOptions[0]?.textContent.includes("Approved"), 15000);
  await until("the turn to end", () => !withText(".composer-actions button", "Stop", pane), 15000);
  withText(".panel-tabs [role=tab]", "Info").click();
  const permissions = await until("the mode", () => [...document.querySelectorAll("aside.panel .panel-info > div")].find((d) => d.querySelector("dt")?.textContent === "Permissions"));
  check("approving goes on in the mode chosen", /accept/i.test(permissions.querySelector("dd").textContent), permissions.querySelector("dd").textContent);
  withText(".panel-tabs [role=tab]", "Approvals")?.click();
});

await step("an agent asks with a form", async () => {
  openSession("Check plan");
  await until("Check plan", () => activePane().querySelector(".session-titleline h2")?.textContent === "Check plan");
  const pane = activePane();
  const box = await until("the message box", () => pane.querySelector(".composer textarea"));
  box.focus();
  type(box, "[ask] the tickets");
  await sleep(80);
  (await until("Send", () => { const b = pane.querySelector(".composer-send"); return b && !b.disabled && b; })).click();
  const form = await until("the questions", () => pane.querySelector(".approval .mcp-form"), 20000);
  const card = form.closest("details");
  check("a question is no input to read", !card.querySelector(".tool-input"));
  const radios = form.querySelectorAll('input[type="radio"]');
  const boxes = form.querySelectorAll('input[type="checkbox"]');
  check("a choice reads as a list, with what each means", radios.length === 2 && form.textContent.includes("You paste all tickets first"), `${radios.length} radios`);
  check("several can be picked where the agent allows it", boxes.length === 3, `${boxes.length} boxes`);
  const buttons = [...form.closest(".approval").querySelectorAll(".approval-buttons button")].map((b) => b.textContent.trim());
  check("a form is sent or skipped", buttons.includes("Send") && buttons.includes("Skip"), buttons.join(", "));
  withText(".mcp-choice", "One by one", form).querySelector("input").click();
  withText(".mcp-choice", "Tests", form).querySelector("input").click();
  withText(".mcp-choice", "Types", form).querySelector("input").click();
  const other = [...form.querySelectorAll('input[type="text"]')][0];
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(other, "start with the backend");
  other.dispatchEvent(new Event("input", { bubbles: true }));
  await sleep(80);
  withText(".approval-buttons button", "Send", form.closest(".approval")).click();
  // The reply streams in a word at a time: wait for its end.
  let reply = null;
  try {
    reply = await until("the agent's answer", () => all(".msg-agent, .md", pane).find((e) => e.textContent.includes("Thanks.") && e.textContent.includes("Tests, Types")), 20000);
  } catch {
    reply = all(".msg-agent, .md", pane).find((e) => e.textContent.includes("Thanks.")) ?? pane.querySelector(".conversation");
  }
  check("the answers reach the agent", reply.textContent.includes("One by one (note: start with the backend)") && reply.textContent.includes("Tests, Types"), reply.textContent.slice(0, 200));
  check("the questions stay on the card", card.querySelectorAll(".agent-questions li").length === 2);
});

await step("the usage pane cycles through its views", async () => {
  document.querySelector('.layout-switch button[aria-label="4 panes"]').click();
  await sleep(300);
  // An empty box's height leaves out its placeholder, so one in a pane the user is not in
  // is short and on one line (GUI-042).
  const idle = await until("a pane not in use", () => document.querySelector(".pane-split:not(.pane-active) .composer textarea"));
  // Measured in the box's own font, since WebKit does not report a placeholder's style.
  const font = getComputedStyle(idle);
  const pen = document.createElement("canvas").getContext("2d");
  pen.font = `${font.fontStyle} ${font.fontWeight} ${font.fontSize} ${font.fontFamily}`;
  const room = idle.clientWidth - parseFloat(font.paddingLeft) - parseFloat(font.paddingRight);
  const needs = pen.measureText(idle.placeholder).width;
  check("a pane not in use shows its placeholder whole", idle.value !== "" || needs <= room, `"${idle.placeholder}" needs ${Math.round(needs)} px of ${Math.round(room)}`);
  let usage = document.querySelector(".pane-usage");
  if (!usage) {
    (await until("an empty pane's Show usage", () => withText(".pane-empty button", "Show usage"))).click();
    usage = await until("the usage pane", () => document.querySelector(".pane-usage"));
  }
  const name = () => usage.querySelector(".usage-view-name")?.textContent;
  const arrow = (which) => usage.querySelector(`button[aria-label^="${which} view"]`);
  const names = ["Recent hours", "Last 12 months", "Tokens", "By model"];
  const seen = [name()];
  for (let i = 0; i < names.length; i++) {
    arrow("Next").click();
    await sleep(150);
    seen.push(name());
  }
  const start = names.indexOf(seen[0]);
  check("the arrows reach every view and come round", start >= 0 && seen.every((s, i) => s === names[(start + i) % names.length]), seen.join(" > "));
  arrow("Previous").click();
  await sleep(150);
  check("the left arrow goes back", name() === names[(start + names.length - 1) % names.length], name());
  const to = async (label) => {
    for (let i = 0; i < names.length && name() !== label; i++) {
      arrow("Next").click();
      await sleep(150);
    }
  };
  await to("Last 12 months");
  const scroller = await until("the year grid", () => usage.querySelector(".usage-calendar-scroll"));
  await until("the days", () => scroller.querySelector(".usage-calendar-day"));
  await sleep(200);
  check("the year opens on today", scroller.scrollWidth <= scroller.clientWidth || scroller.scrollLeft + scroller.clientWidth >= scroller.scrollWidth - 2, `${scroller.scrollLeft} of ${scroller.scrollWidth - scroller.clientWidth}`);
  check("the year stays in its pane", scroller.getBoundingClientRect().right <= usage.getBoundingClientRect().right + 1);
  check("the pane leaves the days as a table to the page", !usage.querySelector(".usage-calendar-table"));
  await to("Tokens");
  const paneTotal = (await until("the pane's tokens", () => usage.querySelector(".usage-hero strong")))?.textContent;
  check("the tokens view has its figures", /\d/.test(paneTotal) && usage.querySelectorAll(".usage-tile").length >= 3, paneTotal);
  await to("By model");
  await until("the models", () => usage.querySelector(".pane-shares li"));
  check("the models view lists models and providers", usage.querySelectorAll(".pane-shares").length === 2);
  // The keys, from the header.
  arrow("Next").focus();
  arrow("Next").dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowLeft", bubbles: true }));
  await sleep(150);
  check("the left key goes back a view", name() === "Tokens", name());
  // GUI-042: in a pane of four, Recent hours keeps its controls to one row, its times
  // apart, and a filter in view.
  await to("Recent hours");
  const bar = await until("the controls", () => usage.querySelector(".live-usage-bar"));
  const tops = [...bar.children].map((c) => Math.round(c.getBoundingClientRect().top));
  check("the pane's controls are one row", tops.every((t) => Math.abs(t - tops[0]) <= 4) && [...usage.querySelectorAll(".live-usage-controls select")].every((sel) => sel.closest(".usage-filter-menu")), tops.join(", "));
  await until("the time axis", () => usage.querySelector("text.usage-axis"));
  // The times are the bottom row; the values share the class up the left side.
  const axis = [...usage.querySelectorAll("text.usage-axis")].map((t) => t.getBoundingClientRect());
  const bottom = Math.max(...axis.map((r) => r.top));
  const labels = axis.filter((r) => Math.abs(r.top - bottom) < 2).sort((x, y) => x.left - y.left);
  check("the pane's times do not touch", labels.length > 1 && labels.every((r, i) => i === 0 || r.left >= labels[i - 1].right + 4), labels.map((r) => `${Math.round(r.left)}-${Math.round(r.right)}`).join(" "));
  const menu = bar.querySelector(".usage-filter-menu");
  menu.querySelector("summary").click();
  await until("the filter menu", () => menu.open);
  const project = menu.querySelector("select[aria-label^='Filter:']");
  if (project.options.length > 1) {
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(project, project.options[1].value);
    project.dispatchEvent(new Event("change", { bubbles: true }));
    const chip = await until("the filter's chip", () => usage.querySelector(".usage-chips .chip"));
    check("a filter shows as a chip", chip.textContent.includes(project.options[1].value) && /1/.test(menu.querySelector(".filter-count")?.textContent ?? ""));
    chip.click();
    await until("the chip gone", () => !usage.querySelector(".usage-chips .chip"));
  }
  document.body.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
  await sleep(50);
  check("the filter menu closes on a click elsewhere", !menu.open);
  // The same figures as the usage page over the same 30 days.
  usage.querySelector('button[aria-label="Full usage page"]').click();
  const pageTotal = (await until("the usage page's tokens", () => document.querySelector(".usage-page .usage-hero strong"), 20000))?.textContent;
  check("the pane and the usage page agree", paneTotal === pageTotal, `${paneTotal} and ${pageTotal}`);
  openSession("Check alpha");
  await until("the workspace", () => document.querySelector(".pane"));
  document.querySelector('.layout-switch button[aria-label="One pane"]')?.click();
  await sleep(200);
});

await step("the bypass setting", async () => {
  document.querySelector(".settings-link").click();
  (await until("the Permissions page", () => withText(".settings-nav button", "Permissions"))).click();
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
  withText(".settings-nav button", "Permissions").click();
  await until("the setting", box);
  check("the setting survives leaving the page", box().checked);
  window.fetch = real;
});

return results;
