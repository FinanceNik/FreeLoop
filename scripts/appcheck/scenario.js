// The app check on fresh scratch data (scripts/appcheck.sh); helpers are in lib.js.
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
  // In its own colour, the --git token Settings → Appearance can change.
  const probe = document.body.appendChild(document.createElement("span"));
  probe.style.color = "var(--git)";
  const gitColour = getComputedStyle(probe).color;
  probe.remove();
  check("the git mark has its own colour", git && getComputedStyle(git).color === gitColour && gitColour !== getComputedStyle(withText(".session-row", "Check alpha").querySelector(".session-title")).color, `${git && getComputedStyle(git).color} and ${gitColour}`);
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
  // DIAGNOSIS (CI): how long opening a session takes there, which once ran past 15 s.
  const opening = Date.now();
  await until("the header", () => document.querySelector(".session-header h2")?.textContent === "Check alpha" && document.querySelector(".session-header .source-mark"), 30000);
  results.push({ name: "DIAGNOSIS opening a session", ok: true, detail: `${Date.now() - opening} ms` });
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
// The sidebar's footer: the four layout icons sit under the four icons at the left of the
// row above them, whether or not the bell is shown, and Help and Settings stack at the
// right.
await step("the footer's icons line up, with Settings at the lower right", async () => {
  const centre = (e) => { const r = e.getBoundingClientRect(); return Math.round(r.left + r.width / 2); };
  const icons = [...document.querySelector(".footer-links").children].filter((e) => !e.classList.contains("footer-cost"));
  const top = icons.slice(0, 4);
  const layout = [...document.querySelectorAll(".layout-switch button")];
  check("four icons and Help above, four layout icons below", icons.length === 5 && icons[4].classList.contains("footer-help") && layout.length === 4, `${icons.length} and ${layout.length}`);
  check("each layout icon is under the one above it", top.every((e, i) => Math.abs(centre(e) - centre(layout[i])) <= 1), `${top.map(centre)} vs ${layout.map(centre)}`);
  const settings = document.querySelector(".footer-row .settings-link").getBoundingClientRect();
  const row = document.querySelector(".footer-row").getBoundingClientRect();
  check("Settings is at the right end of the bottom row, after the layout icons", settings.left >= layout[3].getBoundingClientRect().right && Math.abs(settings.right - row.right) <= 2, `${settings.left}-${settings.right} in ${row.right}`);
  const help = document.querySelector(".footer-help").getBoundingClientRect();
  check("Help is right over Settings", Math.abs(help.right - settings.right) <= 1 && help.bottom <= settings.top, JSON.stringify([help.right, settings.right]));
  const cost = document.querySelector(".footer-cost")?.getBoundingClientRect();
  if (cost) check("Settings is under the cost", cost.right <= settings.right + 12 && cost.left < settings.right && cost.bottom <= settings.top, JSON.stringify([cost.left, cost.right, settings.left, settings.right]));
});

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
  check("a session's context shows on its pane", /^[\d.]+[kM]? · (<1|\d+) %$/.test(low.textContent) && !low.classList.contains("context-high"), low.textContent);
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
  check("a full context stands out", /^[\d.]+[kM]? · 8\d %$/.test(meter().textContent), meter().textContent);
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
  await until("the share of the new limit", () => Number(/ · (\d+) %$/.exec(meter()?.textContent ?? "")?.[1] ?? 0) >= 5, 15000);
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

// Opening a long transcript lands on its end and stays there while it settles. No write
// moves it away from the end on the way, not even one put right later in the frame: WebKit
// draws nothing where the view was sent, so the conversation went blank in every such
// frame, for as long as its items took to draw (9 s for 3,900 of them).
await step("a long transcript opens at its end", async () => {
  const native = Object.getOwnPropertyDescriptor(Element.prototype, "scrollTop");
  let away = 0;
  let writes = 0;
  Object.defineProperty(Element.prototype, "scrollTop", {
    configurable: true,
    get() {
      return native.get.call(this);
    },
    set(v) {
      const atEnd = this.classList?.contains("conversation") && toTheEnd(this) <= 60;
      native.set.call(this, v);
      if (!atEnd) return;
      writes++;
      away = Math.max(away, toTheEnd(this));
    },
  });
  try {
    openSession("Check long");
    const c = await until("the conversation", () => { const x = document.querySelector(".conversation"); return x && x.scrollHeight > x.clientHeight && x; });
    const seen = [];
    for (let i = 0; i < 30; i++) {
      await sleep(100);
      seen.push(Math.round(toTheEnd(c)));
    }
    check("a long transcript opens at its end", toTheEnd(c) <= 60, `${c.children.length} items, px from the end over 3 s: ${seen.filter((_, i) => i % 5 === 0).join(" ")} ... ${seen.at(-1)}`);
    check("a long transcript is not moved from its end while it is drawn", away <= 60, `${writes} writes at the end, one left it ${Math.round(away)} px away`);
  } finally {
    Object.defineProperty(Element.prototype, "scrollTop", native);
  }
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

// Events reach the window in batches, one per frame (NFR-PERF-001): a turn's chunks
// redraw the app far less often than there are events.
await step("a turn's events redraw the app once per batch", async () => {
  openSession("Check alpha");
  await until("the conversation", () => document.querySelector(".conversation"));
  window.__freeloopCount = { renders: 0, events: 0 };
  await turn("Check alpha");
  await sleep(200);
  const { renders, events } = window.__freeloopCount;
  delete window.__freeloopCount;
  check("the turn made events", events >= 10, `${events} events`);
  check("the app rendered less than once for every three events", renders * 3 <= events, `${renders} renders for ${events} events`);
});

// A session in no pane holds no events (NFR-PERF-004): a turn running in one that was
// left shows in its row, and opening it again shows the turn as it stands.
await step("a session left in every pane gives up its events, and opening it shows them again", async () => {
  window.__freeloopCount = { renders: 0, events: 0, letGoMs: 1000 };
  try {
    openSession("Check alpha");
    await until("the conversation", () => document.querySelector(".conversation"));
    const box = await until("the message box", () => activePane().querySelector(".composer textarea"));
    type(box, "go");
    await sleep(50);
    (await until("Send to be enabled", () => { const b = named(".composer-actions button.primary", "Send", activePane()); return b && !b.disabled && b; })).click();
    openSession("Check beta");
    await until("the row to say what alpha waits for", () => withText(".session-row", "Check alpha")?.title);
    check("the sidebar knows what a session left on screen waits for", true, withText(".session-row", "Check alpha").title);
    await sleep(2000);
    const { events, held } = window.__freeloopCount;
    check("only the session on screen holds events", held <= 1, `${held} sessions hold events, ${events} events arrived`);
    openSession("Check alpha");
    await until("the approval in the reopened session", () => activePane().querySelector(".approval-buttons button.primary"), 30000);
    check("the reopened session shows its running turn", true);
    const ended = () => all(".turn-stop", activePane()).length;
    const turns = ended();
    await until("the turn to end", () => {
      activePane().querySelector(".approval-buttons button.primary")?.click();
      return ended() > turns;
    }, 30000);
    check("the reopened session's turn ends as it would have", true);
  } finally {
    delete window.__freeloopCount;
  }
});

await step("a session opens", async () => {
  openSession("Check alpha");
  await until("the conversation", () => document.querySelector(".conversation"));
  check("a session opens", true);
});

// The count of queued messages sits on the Send button: it never makes the row taller.
await step("a queued message leaves the composer's size alone", async () => {
  const pane = activePane();
  const actions = await until("the composer's row", () => pane.querySelector(".composer-actions"));
  const wrap = pane.querySelector(".composer-send-wrap");
  const before = pane.querySelector(".composer").getBoundingClientRect().height;
  const badge = document.createElement("span");
  badge.className = "composer-queued";
  badge.textContent = "12";
  wrap.appendChild(badge);
  const after = pane.querySelector(".composer").getBoundingClientRect().height;
  const b = badge.getBoundingClientRect();
  const send = wrap.querySelector(".composer-send").getBoundingClientRect();
  badge.remove();
  check("a queued count changes no height", after === before, `${before} → ${after}`);
  check("the count sits on the Send button's corner", b.right > send.right - 4 && b.top < send.top + 4 && b.width < 30, JSON.stringify({ b: [b.left, b.top, b.width], send: [send.right, send.top] }));
  check("the row did not wrap", actions.getBoundingClientRect().height < 60, actions.getBoundingClientRect().height);
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
  // Rows are flat: no outline, a background only on the open one, the agent's mark
  // quiet until its row is open, and no status pill or line of tags.
  const flat = getComputedStyle(document.querySelector(".session-row:not([aria-current]):not(:hover)"));
  check("rows have no outline", flat.borderTopColor === "rgba(0, 0, 0, 0)" && flat.backgroundColor === "rgba(0, 0, 0, 0)", `${flat.borderTopColor}, ${flat.backgroundColor}`);
  const open = document.querySelector('.session-row[aria-current="true"]');
  check("the open session's row stands out", open && getComputedStyle(open).backgroundColor !== "rgba(0, 0, 0, 0)");
  const quiet = document.querySelector(".session-row:not([aria-current]):not(:hover) .source-mark");
  check("other rows' marks are quiet, the open one's in colour", quiet && getComputedStyle(quiet).filter !== "none" && getComputedStyle(open.querySelector(".source-mark")).filter === "none", quiet && getComputedStyle(quiet).filter);
  check("no row has a status pill or a line of tags", !document.querySelector(".session-row .badge-running, .session-row .badge-waiting_permission, .session-row .tag, .session-row .session-meta"));
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
  check("a waiting session's row has a dot saying so", waitingDot === "Needs you", `it said "${waitingDot}"`);
});

// GUI-049: what was typed before comes back without a model: the grey rest of an
// earlier prompt, ↑ in an empty box and a search from the palette; and @ puts the
// file the session just wrote first.
await step("earlier prompts come back, and @ puts the session's files first", async () => {
  await turn("Check alpha", () => {}, "go and read the notes");
  const pane = activePane();
  const box = pane.querySelector(".composer textarea");
  const key = (k, o = {}) => box.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, cancelable: true, ...o }));
  const kept = JSON.parse(localStorage.getItem("freeloop.history") ?? "[]");
  check("a sent prompt is remembered", kept[0]?.text === "go and read the notes", JSON.stringify(kept[0]));
  box.focus();
  await sleep(50);
  key("ArrowUp");
  await sleep(150);
  check("↑ in an empty box recalls it", box.value === "go and read the notes", `it reads "${box.value}"`);
  key("ArrowDown");
  await sleep(150);
  check("↓ past the newest gives the empty box back", box.value === "", `it reads "${box.value}"`);
  type(box, "go and");
  const ghost = await until("the grey rest", () => pane.querySelector(".composer-ghost"), 3000);
  check("the grey text is the rest of the earlier prompt", ghost.textContent === "go and read the notes", ghost.textContent);
  check("it is hidden from screen readers, and the box says it completes inline", ghost.getAttribute("aria-hidden") === "true" && box.getAttribute("aria-autocomplete") === "both");
  const typed = ghost.querySelector(".composer-ghost-typed").getBoundingClientRect();
  const boxAt = box.getBoundingClientRect();
  check("the grey text starts where the box's text does", Math.abs(typed.left - boxAt.left - parseFloat(getComputedStyle(box).paddingLeft)) < 1 && Math.abs(typed.top - boxAt.top - parseFloat(getComputedStyle(box).paddingTop)) < 3, `${typed.left - boxAt.left}, ${typed.top - boxAt.top}`);
  key("Escape");
  await sleep(100);
  check("Escape hides it", !pane.querySelector(".composer-ghost") && box.value === "go and");
  type(box, "go and ");
  await until("the grey rest again", () => pane.querySelector(".composer-ghost"), 3000);
  key("ArrowRight");
  await sleep(150);
  check("→ takes it", box.value === "go and read the notes", `it reads "${box.value}"`);
  check("and the grey text goes", !pane.querySelector(".composer-ghost"));
  type(box, "");
  await sleep(100);
  await openPalette("search earlier prompts");
  (await until("the palette entry", () => withText(".palette-item", "Search earlier prompts"), 3000)).click();
  const list = await until("the earlier prompts", () => pane.querySelector('.mention-picker[aria-label="Earlier prompts"]'), 3000);
  check("the search lists it, with its project", !!withText("li", "go and read the notes", list) && list.querySelector("li .muted")?.textContent.length > 0);
  check("the focus is in the box, for the arrows and Enter", document.activeElement === box);
  key("Enter");
  await sleep(150);
  check("Enter puts it in the box, not sent", box.value === "go and read the notes" && !pane.querySelector('.mention-picker[aria-label="Earlier prompts"]'), `it reads "${box.value}"`);
  type(box, "@");
  box.setSelectionRange(1, 1);
  box.dispatchEvent(new Event("select", { bubbles: true }));
  const files = await until("the file list", () => { const l = pane.querySelector('.mention-picker[aria-label="Files"]'); return l?.querySelector("[role=option]") && l; }, 4000);
  const first = files.querySelector("[role=option] .muted")?.textContent;
  check("@ lists the file the session wrote first", first === "freeloop-demo.txt", first);
  type(box, "");
  await sleep(100);
});

await step("voice input downloads its model once, then dictates at the caret", async () => {
  const pane = activePane();
  const box = pane.querySelector(".composer textarea");
  const mic = () => pane.querySelector("button.composer-dictate");
  const shortcut = () => document.dispatchEvent(new KeyboardEvent("keydown", { key: "D", metaKey: true, shiftKey: true, bubbles: true, cancelable: true }));
  type(box, "Fix it");
  box.setSelectionRange(3, 3);
  await until("the mic", mic, 3000);
  check("the mic is named, with its shortcut", mic().getAttribute("aria-label") === "Dictate" && mic().getAttribute("aria-keyshortcuts") === "Meta+Shift+D" && mic().title.includes("D"), mic().title);
  mic().click();
  const named = '[role=dialog][aria-label="Download the speech model"]';
  const dialog = await until("the download dialog", () => document.querySelector(named), 3000);
  check("before downloading, it says what and from where", /Hugging Face/.test(dialog.textContent) && /\d MB/.test(dialog.textContent) && /MIT/.test(dialog.textContent), dialog.textContent);
  withText("button", "Download", dialog).click();
  await until("the dialog to close once the model is there", () => !document.querySelector(named), 6000);
  mic().click();
  await until("recording", () => mic().getAttribute("aria-pressed") === "true", 3000);
  await sleep(1200);
  check("while it records, the time shows and the mic stops it", /^0:0[1-2]$/.test(pane.querySelector(".dictation-clock")?.textContent ?? "") && mic().getAttribute("aria-label") === "Stop and insert the words", pane.querySelector(".dictation-clock")?.textContent);
  mic().click();
  await until("the words", () => /Dictated/.test(box.value), 5000);
  check("the words go in at the caret, with spaces, and nothing is sent", /^Fix Dictated \d\.\d seconds\. it$/.test(box.value), box.value);
  shortcut();
  await until("recording from the shortcut", () => mic().getAttribute("aria-pressed") === "true", 3000);
  document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
  await sleep(300);
  check("Escape throws the recording away", mic().getAttribute("aria-pressed") === "false" && /^Fix Dictated \d\.\d seconds\. it$/.test(box.value), box.value);
  type(box, "");
  await sleep(100);
});

await step("the phone page turns the phone server on, and pairs only with a code", async () => {
  await openPalette("phone");
  (await until("the palette entry", () => all(".palette-item").find((e) => e.textContent.trim().startsWith("Phone")), 3000)).click();
  const page = await until("the phone page", () => all(".page").find((p) => p.querySelector("h2")?.textContent === "Phone"), 5000);
  const status = () => page.querySelector('[role="status"]')?.textContent ?? "";
  const listed = await until("the paired phone", () => [...page.querySelectorAll(".phone-devices li")].find((li) => li.querySelector("input")?.value === "Check phone"), 5000);
  check("the list says the phone gets notifications", listed.textContent.includes("notifications on"), listed.textContent);
  const enable = withText("label", "Answer my phone", page).querySelector("input");
  enable.click();
  await until("the server on", () => /^On, on this computer's port \d+\./.test(status()), 8000);
  check("it says it answers on this computer's port", /^On, on this computer's port \d+\./.test(status()), status());
  await until("the sidebar's word", () => withText("button.link.today", "Phone on"), 15000).catch(() => null);
  check("the sidebar says the phone is on", !!withText("button.link.today", "Phone on"));
  const start = () => all("button", page).find((b) => b.textContent === "Pair a phone…");
  check("pairing waits for the name the phone reaches this computer by", start()?.disabled === true);
  const host = withText("label", "The name your phone reaches", page).querySelector("input");
  host.focus();
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(host, "phone.test");
  host.dispatchEvent(new Event("input", { bubbles: true }));
  host.blur();
  await until("pairing offered", () => start() && !start().disabled, 8000);
  start().click();
  const code = await until("the code", () => page.querySelector(".phone-code")?.textContent, 5000);
  check("the code is 20 letters and digits, in groups", /^[0-9A-Z]{4}(-[0-9A-Z]{4}){4}$/.test(code), code);
  check("the address carries the code", page.querySelector(".phone-link-text")?.textContent === `https://phone.test:8443/phone/#pair=${code}`, page.querySelector(".phone-link-text")?.textContent);
  withText("button", "Cancel", page).click();
  await until("the window closed", () => !page.querySelector(".phone-code"), 5000);
  enable.click();
  await until("the server off", () => status() === "Off.", 8000);
  check("turned off, it says so", status() === "Off.", status());
  document.querySelector(".settings-nav .settings-back").click();
  await sleep(200);
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
    const [px, py] = at(el, x, y);
    // A point outside the window reaches nothing, and the check would say nothing true.
    if (px < 0 || py < 0 || px > innerWidth || py > innerHeight) throw new Error(`${name} is out of sight, at ${Math.round(px)},${Math.round(py)}`);
    await mouse(px, py, 1);
    await sleep(150);
    check(`a click clears a selection: ${name}`, selected() === 0, `${selected()} characters stayed selected`);
  };
  const msg = all(".msg-agent", activePane()).at(-1);
  // On a machine that never shows the window (CI), messages out of sight keep their
  // placeholder heights, so the last one need not be where the view ends: bring it in.
  // Only a reader's input stops the view following the end, so scroll as one would.
  // The conversation alone: scrollIntoView would move the page around it too.
  const c = conversation();
  c.dispatchEvent(new WheelEvent("wheel", { deltaY: -200, bubbles: true }));
  c.scrollTop += msg.getBoundingClientRect().top - c.getBoundingClientRect().top - c.clientHeight / 2;
  await sleep(300);
  await clicked("text", msg);
  await clicked("the conversation's margin", conversation(), 0.01, 0.5);
  await clicked("the header's title", document.querySelector(".session-header h2"));
  const nav = document.querySelector(".sidebar nav");
  // Empty space below the last row, when the list leaves any: a click on a row opens it.
  const rows = [...nav.querySelectorAll(".session-row")];
  const free = nav.getBoundingClientRect().bottom - (rows.at(-1)?.getBoundingClientRect().bottom ?? 0);
  if (free >= 8) await clicked("the sidebar's empty space", nav, 0.5, 1 - 3 / nav.getBoundingClientRect().height);
  // The gap between the icons at the left and Help at the right end.
  const links = document.querySelector(".footer-links");
  const lr = links.getBoundingClientRect();
  const gapLeft = [...links.children].filter((e) => !e.classList.contains("footer-help") && !e.classList.contains("footer-cost")).pop().getBoundingClientRect().right;
  const gapRight = (links.querySelector(".footer-cost") ?? links.querySelector(".footer-help")).getBoundingClientRect().left;
  await clicked("the footer's empty space", links, (((gapLeft + gapRight) / 2) - lr.left) / lr.width, 0.5);
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
  // The queue has an icon of its own in the sidebar (GUI-016), which says how many tasks wait.
  const icon = () => all(".footer-links button").find((b) => /^Queue\b/.test(b.getAttribute("aria-label") ?? ""));
  check("the sidebar has a Queue icon", !!icon() && icon().getAttribute("aria-label") === "Queue" && !icon().querySelector(".footer-badge"), icon()?.getAttribute("aria-label"));
  icon().click();
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
  const counted = await until("the icon's count", () => icon()?.querySelector(".footer-badge")?.textContent === "1" && icon(), 9000).catch(() => null);
  check("the Queue icon counts the waiting task", !!counted && /1 task waiting/.test(counted.getAttribute("aria-label")), icon()?.getAttribute("aria-label"));
  row.querySelector('button[aria-label^="Remove"]').click();
  await until("the task removed", () => !page.textContent.includes("Write a haiku"), 5000);
  await until("the icon's count gone", () => icon()?.getAttribute("aria-label") === "Queue", 9000).catch(() => {});
  check("the Queue icon is plain again", icon()?.getAttribute("aria-label") === "Queue" && !icon().querySelector(".footer-badge"), icon()?.getAttribute("aria-label"));
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

// The Board is for workflow issues only (GUI-016): ordinary sessions stay in the sidebar
// and Needs attention, and with no backlog the Board says so.
await step("the Board shows no ordinary session, and an untrusted project's row reads nothing", async () => {
  named(".footer-links button", "Board").click();
  const page = await until("the Board", () => document.querySelector(".board-page"), 8000);
  check("no ordinary session is on the Board", !withText(".board-card", "Check") && !page.textContent.includes("Check alpha") && !page.textContent.includes("Check long"), page.textContent.slice(0, 120));
  check("the old session columns are gone", all(".board-col h3, .board-column h3", page).length === 0 && !/Needs you|Running|Idle/.test(page.textContent), page.textContent.slice(0, 120));
  // The project with a backlog folder that is not trusted yet has a row to trust it by, and
  // nothing in its folder is read for it (SEC-006).
  const row = await until("the untrusted project's row", () => page.querySelector('.board-lane-untrusted[aria-label="Backlog of pile"]'), 15000);
  check("an untrusted project's row says so and offers to trust it", row.textContent.includes("Not trusted: trust it to see its backlog.") && !!withText("button", "Trust project", row), row.textContent.slice(0, 100));
  check("nothing in its backlog is read or shown", !page.textContent.includes("Finished issue") && all(".board-col", row).length === 0, row.textContent.slice(0, 100));
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
  // What waits for Save survives a visit to another page (SET-007).
  withText(".settings-nav button", "Costs").click();
  (await until("Add a budget", () => withText("button", "Add a budget"))).click();
  await until("the Save bar", () => document.querySelector(".settings-save"));
  withText(".settings-nav button", "Providers").click();
  await until("the Providers page", () => document.querySelector(".settings-page h2")?.textContent !== "Costs");
  withText(".settings-nav button", "Costs").click();
  await until("Costs again", () => document.querySelector(".settings-page h2")?.textContent === "Costs");
  check("an unsaved budget survives another page", !!document.querySelector(".budget-row") && !!document.querySelector(".settings-save"));
  withText(".settings-save button", "Revert")?.click();
  await until("reverted", () => !document.querySelector(".budget-row"), 4000).catch(() => {});
  document.querySelector(".settings-nav .settings-back").click();
  await sleep(100);
});

await step("a style keeps the fonts and colors, and choosing it brings them back", async () => {
  // GUI-053: saved from what is on screen, switched to from the list and the
  // palette, marked changed, reverted, and deleted with the look left as it is.
  document.querySelector(".settings-link").click();
  (await until("the Appearance page", () => withText(".settings-nav button", "Appearance"))).click();
  await until("Appearance", () => document.querySelector(".settings-page h2")?.textContent === "Appearance");
  const picker = await until("the styles", () => document.querySelector(".style-picker"));
  const select = () => picker.querySelector("select.style-select");
  const shown = () => select().selectedOptions[0]?.textContent ?? "";
  const accent = () => getComputedStyle(document.documentElement).getPropertyValue("--accent").trim();
  const setText = (input, value) => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
  };
  const pick = (value) => {
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(select(), value);
    select().dispatchEvent(new Event("change", { bubbles: true }));
  };
  // A served build has no native dialogs, so this answers them.
  let answer = "Cancel";
  let asked = 0;
  const real = window.fetch;
  window.fetch = (u, o) => {
    const text = decodeURIComponent(`${u}${o?.body ?? ""}`);
    if (text.includes("not saved in a style") || text.includes("Delete the style Check paper?")) {
      asked++;
      return Promise.resolve(new Response(JSON.stringify(answer), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    return real(u, o);
  };
  try {
    const own = accent();
    check("the app's own look is chosen at first", select().value === "" && shown() === "Freeloop (default)", shown());
    setText(named(".color-editor input.color-text", "Buttons value"), "#d14b8f");
    await until("the new accent", () => accent() === "#d14b8f", 3000);
    check("a color of the user's marks the app's own look changed", shown() === "Freeloop (default) (changed)", shown());

    withText(".style-picker button", "Save as new…").click();
    setText(await until("the name field", () => picker.querySelector("input.style-name")), "Check paper");
    withText(".style-picker button", "Save").click();
    await until("the new style chosen", () => shown() === "Check paper", 5000);
    const id = select().value;
    check("saving as new chooses the style", id === "check-paper", id);

    pick("");
    await until("the app's own accent", () => accent() === own, 3000);
    check("the app's own look takes the colors away without asking", asked === 0 && select().value === "", `asked ${asked}`);
    pick(id);
    await until("the style's accent", () => accent() === "#d14b8f", 3000);
    check("choosing the style brings its colors back", shown() === "Check paper", shown());

    setText(named(".color-editor input.color-text", "Buttons value"), "#1f9d55");
    await until("the changed mark", () => shown() === "Check paper (changed)", 3000);
    check("a change offers Update and Revert", !!withText(".style-picker button", "Update style") && !!withText(".style-picker button", "Revert"));
    await sleep(500);
    pick("");
    await sleep(600);
    check("switching away from a change asks first", asked === 1, `asked ${asked} times`);
    check("declining keeps the change", accent() === "#1f9d55" && shown() === "Check paper (changed)", `${accent()} ${shown()}`);
    withText(".style-picker button", "Revert").click();
    await until("the style's accent again", () => accent() === "#d14b8f", 3000);
    check("Revert goes back to the style", shown() === "Check paper", shown());

    pick("");
    await until("the app's own accent", () => accent() === own, 3000);
    await openPalette("Style: Check paper");
    (await until("the palette entry", () => all(".palette-item").find((i) => i.querySelector(".palette-title")?.textContent === "Style: Check paper"), 3000)).click();
    await until("the style from the palette", () => accent() === "#d14b8f", 3000);
    check("the palette switches to a style", shown() === "Check paper", shown());

    answer = "OK";
    withText(".style-picker button", "Delete style").click();
    await until("the style gone", () => !all("option", select()).some((o) => o.value === id), 5000);
    check("deleting leaves the look as it is", accent() === "#d14b8f" && shown() === "Freeloop (default) (changed)", `${accent()} ${shown()}`);
  } finally {
    window.fetch = real;
  }
  // Leave the app's own colors for the steps after this one.
  all(".color-editor button").find((b) => /^Reset (light|dark) colors$/.test(b.textContent) && !b.disabled)?.click();
  await until("the app's own colors", () => shown() === "Freeloop (default)", 3000).catch(() => {});
  await sleep(500);
  document.querySelector(".settings-nav .settings-back").click();
  await sleep(100);
});

await step("the Settings page follows the language chosen, at once", async () => {
  // GUI-048: picked beside Theme, it applies without a reload, and the page's lang follows.
  document.querySelector(".settings-link").click();
  (await until("the Appearance page", () => withText(".settings-nav button", "Appearance"))).click();
  await until("Appearance", () => document.querySelector(".settings-page h2")?.textContent === "Appearance");
  const pick = (value) => {
    const select = document.querySelector(".settings-page select.language-select");
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(select, value);
    select.dispatchEvent(new Event("change", { bubbles: true }));
  };
  check("the language is picked beside Theme", document.querySelector(".settings-page select.language-select")?.closest("label")?.previousElementSibling?.textContent.startsWith("Theme"));
  pick("de");
  const h2 = await until("the heading in German", () => document.querySelector(".settings-page h2")?.textContent === "Darstellung" && document.querySelector(".settings-page h2"), 8000).catch(() => null);
  check("Deutsch turns the Settings heading German without a reload", h2, document.querySelector(".settings-page h2")?.textContent);
  check("the page's lang follows", document.documentElement.lang === "de", document.documentElement.lang);
  check("the sections and the column follow", !!withText(".settings-page h3", "Farbmodus und Textgröße") && !!withText(".settings-nav button", "Zurück") && !!withText(".settings-nav h2", "Modelle"));
  pick("en");
  await until("the heading in English", () => document.querySelector(".settings-page h2")?.textContent === "Appearance", 8000).catch(() => {});
  check("English comes back", document.querySelector(".settings-page h2")?.textContent === "Appearance" && document.documentElement.lang === "en", `${document.querySelector(".settings-page h2")?.textContent} ${document.documentElement.lang}`);
  document.querySelector(".settings-nav .settings-back").click();
  await sleep(100);
});

await step("the session and its composer follow the language, without a reload", async () => {
  // GUI-048: in Deutsch the composer's placeholder and the app's own mode names are
  // German, and the sidebar's landmarks too; English brings them back.
  window.appcheckNotReloaded = true;
  openSession("Check alpha");
  const box = () => activePane().querySelector(".composer textarea");
  await until("the message box", box);
  const modes = () => all("option", activePane().querySelector(".session-control[data-mode] select")).map((o) => o.textContent);
  const nav = () => document.querySelector(".sidebar nav")?.getAttribute("aria-label");
  const english = { placeholder: box().placeholder, modes: modes(), nav: nav() };
  const pick = async (value) => {
    document.querySelector(".settings-link").click();
    (await until("the Appearance page", () => withText(".settings-nav button", "Appearance") ?? withText(".settings-nav button", "Darstellung"))).click();
    const select = await until("the language", () => document.querySelector(".settings-page select.language-select"));
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(select, value);
    select.dispatchEvent(new Event("change", { bubbles: true }));
    await until(`the page in ${value}`, () => document.documentElement.lang === value, 8000);
    document.querySelector(".settings-nav .settings-back").click();
    await until("the session again", box);
  };
  try {
    check("the composer starts in English", english.placeholder.startsWith("Ask for a change") && english.modes.includes("Ask") && english.modes.includes("Plan"), `${english.placeholder}; ${english.modes.join(", ")}`);
    await pick("de");
    const placeholder = box().placeholder;
    check("Deutsch turns the composer's placeholder German", placeholder && placeholder !== english.placeholder && !placeholder.startsWith("Ask") && placeholder.includes("@") && placeholder.includes("/"), placeholder);
    check("the app's own mode names are German", modes().includes("Fragen") && modes().includes("Planen") && !modes().includes("Ask") && !modes().includes("Plan"), modes().join(", "));
    check("the sidebar's landmarks are German", nav() && nav() !== english.nav, nav());
    await pick("en");
    check("English comes back", box().placeholder === english.placeholder && modes().join() === english.modes.join() && nav() === english.nav, `${box().placeholder}; ${modes().join(", ")}; ${nav()}`);
    check("no reload in between", window.appcheckNotReloaded === true);
  } finally {
    // Later steps find English and the session they expect.
    if (document.documentElement.lang !== "en") await pick("en").catch(() => {});
    if (document.querySelector(".settings-nav")) document.querySelector(".settings-nav .settings-back")?.click();
    await sleep(100);
  }
});

await step("the attention view and the Board follow the language, without a reload", async () => {
  // GUI-048: in Nederlands the attention view's and the Board's headings are Dutch, and
  // so are the Board's columns; English brings them back.
  window.appcheckNotReloaded = true;
  const pick = async (value) => {
    document.querySelector(".settings-link").click();
    (await until("the Appearance page", () => withText(".settings-nav button", "Appearance") ?? withText(".settings-nav button", "Weergave"))).click();
    const select = await until("the language", () => document.querySelector(".settings-page select.language-select"));
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(select, value);
    select.dispatchEvent(new Event("change", { bubbles: true }));
    await until(`the page in ${value}`, () => document.documentElement.lang === value, 8000);
    document.querySelector(".settings-nav .settings-back").click();
    await sleep(100);
  };
  const heading = (sel) => document.querySelector(`${sel} h2`)?.textContent ?? "";
  const boardText = () => document.querySelector(".board-page")?.textContent ?? "";
  try {
    await pick("nl");
    (document.querySelector(".attention-link") ?? named(".footer-links button", "Aandacht")).click();
    const attention = await until("the attention view in Dutch", () => heading(".page.attention") === "Vraagt aandacht", 8000).catch(() => false);
    check("Nederlands turns the attention view's heading Dutch", attention, heading(".page.attention"));
    const board = named(".footer-links button", "Bord");
    check("the sidebar names the Board in Dutch", !!board, all(".footer-links button").map((b) => b.getAttribute("aria-label")).join(", "));
    board?.click();
    const shown = await until("the Board in Dutch", () => heading(".board-page") === "Bord", 8000).catch(() => false);
    check("Nederlands turns the Board's heading Dutch", shown, heading(".board-page"));
    check("the Board's words are Dutch", boardText().includes("Niet vertrouwd: vertrouw het project om de backlog te zien."), boardText().slice(0, 100));
    check("the Queue icon is Dutch", !!named(".footer-links button", "Wachtrij"), all(".footer-links button").map((b) => b.getAttribute("aria-label")).join(", "));
    await pick("en");
    named(".footer-links button", "Board").click();
    await until("the Board in English", () => heading(".board-page") === "Board", 8000).catch(() => {});
    check("English comes back", heading(".board-page") === "Board" && boardText().includes("Not trusted: trust it to see its backlog."), `${heading(".board-page")}; ${boardText().slice(0, 80)}`);
    check("no reload in between", window.appcheckNotReloaded === true);
  } finally {
    // Later steps find English and the session they expect.
    if (document.documentElement.lang !== "en") await pick("en").catch(() => {});
    openSession("Check alpha");
    await until("the conversation", () => document.querySelector(".conversation"));
  }
});

await step("a problem report says what an issue needs", async () => {
  // Shown, not copied: a copy here would replace the clipboard of whoever runs the check.
  document.querySelector(".settings-link").click();
  await until("Settings", () => document.querySelector(".settings-nav"));
  withText(".settings-nav button", "Data").click();
  await until("the Data page", () => document.querySelector(".settings-page h2")?.textContent === "Data");
  check("the Data page offers a report", !!withText(".settings-page h3", "Report a problem") && !!withText("button", "Copy a report"));
  // Which build is running (NFR-REL-006): the version and the commit it was made from,
  // so two machines on one version can be compared. The served build is made in this
  // checkout, so its commit is the checkout's, with +dirty when it has changes.
  const about = await until("the build's card", () => withText(".card", "This build"), 5000);
  const line = about.querySelector("p")?.textContent ?? "";
  check("Settings says the version of this build", /^Freeloop \d+\.\d+\.\d+/.test(line), line);
  if (headCommit) check("Settings says the commit this build is from", line.includes(headCommit) && about.querySelector("code")?.textContent.startsWith(headCommit), `${line} (the checkout is at ${headCommit})`);
  const report = await (async () => { withText("button", "Show what is copied").click(); return (await until("the report", () => document.querySelector(".report-preview"), 5000)).textContent; })();
  if (headCommit) check("the report starts with the same commit", report.split("\n")[0].includes(headCommit), report.split("\n")[0]);
  withText("button", "Hide what is copied").click();
  await until("the report hidden", () => !document.querySelector(".report-preview"), 4000);
  withText("button", "Show what is copied").click();
  const pre = await until("the report", () => document.querySelector(".report-preview"), 5000);
  const text = pre.textContent;
  check("the report has the version and the Mac", /^Freeloop \d+\.\d+\.\d+/.test(text) && /System: macOS \d+/.test(text), text.split("\n").slice(0, 3).join(" | "));
  check("the report has the log's end", /The log's last \d+ lines:/.test(text));
  check("the report has no home folder path", !/\/Users\/[^/\s]+\//.test(text), (text.match(/\/Users\/[^/\s]+\//) ?? [""])[0]);
  withText("button", "Hide what is copied").click();
  await until("the report hidden", () => !document.querySelector(".report-preview"), 4000);
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

await step("the Info tab says what the model sees", async () => {
  // NFR-PRIV-003: the system prompt and the tools a built-in session's calls carry.
  const info = async (title) => {
    openSession(title);
    await until(title, () => activePane().querySelector(".session-titleline h2")?.textContent === title);
    const panel = named(".session-actions button", "Panel", activePane());
    if (panel.getAttribute("aria-pressed") !== "true") panel.click();
    (await until("the Info tab", () => withText(".panel-tabs [role=tab]", "Info"))).click();
    return until("what the model sees", () => document.querySelector("aside.panel .model-sees"), 8000);
  };
  const sees = await info("Check alpha");
  const parts = all(".model-sees-part > summary", sees).map((x) => x.textContent);
  check("a built-in session shows its system prompt in parts", parts.some((p) => p.startsWith("Safety")) && parts.some((p) => p.startsWith("Environment")), parts.join(" | "));
  check("and its tools", all(".model-sees-tools code", sees).some((c) => c.textContent === "write_file"));
  check("the prompt is text, not markup", !sees.querySelector(".md, a[href]"));
  const agent = await info("Check agent");
  check("an agent with its own requests says it records none", agent.textContent.includes("Nothing recorded yet") && !agent.querySelector(".model-sees-kind"));
  openSession("Check alpha");
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
  // Nothing is cut off: the pane and its chart card never scroll sideways, and every
  // label is drawn inside the chart.
  const card = usage.querySelector(".usage-card");
  check("the pane and its chart fit their width", usage.scrollWidth <= usage.clientWidth + 1 && card.scrollWidth <= card.clientWidth + 1, `pane ${usage.scrollWidth}/${usage.clientWidth}, card ${card.scrollWidth}/${card.clientWidth}`);
  check("the chart card cuts nothing off", getComputedStyle(card).overflowX === "visible", getComputedStyle(card).overflowX);
  // The controls, the totals and the chart's heading are evenly spaced.
  const strip = usage.querySelector(".live-usage-strip").getBoundingClientRect();
  const above = strip.top - bar.getBoundingClientRect().bottom;
  const below = card.getBoundingClientRect().top - strip.bottom;
  check("the totals sit evenly between the controls and the chart", Math.abs(above - below) <= 1, `${Math.round(above)} px above, ${Math.round(below)} px below`);
  const plot = usage.querySelector(".usage-chart svg").getBoundingClientRect();
  const out = axis.filter((r) => r.left < plot.left - 1 || r.right > plot.right + 1);
  check("the chart's labels are inside it", !out.length, out.map((r) => `${Math.round(r.left)}-${Math.round(r.right)} of ${Math.round(plot.left)}-${Math.round(plot.right)}`).join(" "));
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

await step("the usage pane's numbers follow the language, without a reload", async () => {
  // GUI-048: picked in Settings, Deutsch writes the pane's times 24-hour and its figures
  // in German at once; English brings back what was there. A reload would lose the mark.
  window.appcheckNotReloaded = true;
  document.querySelector('.layout-switch button[aria-label="4 panes"]').click();
  await sleep(300);
  // The step before opened a session over the pane it showed the usage in.
  if (!document.querySelector(".pane-usage")) (await until("an empty pane's Show usage", () => withText(".pane-empty button", "Show usage"))).click();
  await until("the usage pane", () => document.querySelector(".pane-usage"));
  // Settings goes back to the session in use, and the usage pane is none.
  (await until("a session's pane", () => all(".pane").find((p) => p.querySelector(".composer")))).dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
  await sleep(100);
  const recent = async () => {
    const usage = await until("the usage pane", () => document.querySelector(".pane-usage"));
    for (let i = 0; i < 4 && usage.dataset.view !== "recent"; i++) {
      usage.querySelector(".usage-view-step:not(.usage-view-previous)").click();
      await sleep(150);
    }
    await until("the time axis", () => usage.querySelector("text.usage-axis"));
    await sleep(200);
    // The times are the bottom row; the values share the class up the left side.
    const axis = all("text.usage-axis", usage);
    const bottom = Math.max(...axis.map((t) => t.getBoundingClientRect().top));
    const [tokens, cost] = all(".live-usage-strip strong", usage).map((s) => s.textContent);
    return { times: axis.filter((t) => Math.abs(t.getBoundingClientRect().top - bottom) < 2).map((t) => t.textContent), tokens, cost };
  };
  const pick = async (value) => {
    document.querySelector(".settings-link").click();
    (await until("the Appearance page", () => withText(".settings-nav button", "Appearance") ?? withText(".settings-nav button", "Darstellung"))).click();
    const select = await until("the language", () => document.querySelector(".settings-page select.language-select"));
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(select, value);
    select.dispatchEvent(new Event("change", { bubbles: true }));
    await until(`the page in ${value}`, () => document.documentElement.lang === value, 8000);
    document.querySelector(".settings-nav .settings-back").click();
    await until("the workspace", () => document.querySelector(".pane"));
  };
  try {
    const english = await recent();
    await pick("de");
    const german = await recent();
    check("the pane's times are 24-hour in German", german.times.length > 1 && german.times.every((s) => /^\d{1,2}:\d{2}$/.test(s)), german.times.join(" "));
    check("the pane's tokens are written as German writes them", /^-?[\d.]+$|^\d+(,\d)?\s(Mio|Mrd)\.$/.test(german.tokens), `${english.tokens} became ${german.tokens}`);
    check("the pane's cost is dollars as German writes them", !english.cost.includes("$") || /^<?[\d.]+,\d\d\s\$$/.test(german.cost), `${english.cost} became ${german.cost}`);
    await pick("en");
    const again = await recent();
    // By form, not value: the axis may move on a step meanwhile.
    const clock = (times) => [...new Set(times.map((s) => (/[ap]$/.test(s) ? "12-hour" : /^\d{1,2}:\d{2}$/.test(s) ? "24-hour" : s)))].join();
    const unit = (s) => s.replace(/[\d.,]/g, "");
    check("English writes them as before", clock(again.times) === clock(english.times) && unit(again.tokens) === unit(english.tokens), `${english.times.join(" ")}, ${english.tokens}; now ${again.times.join(" ")}, ${again.tokens}`);
    check("no reload in between", window.appcheckNotReloaded === true);
  } finally {
    // Whatever happened, later steps find English, one pane and the session they expect.
    if (document.documentElement.lang !== "en") await pick("en").catch(() => {});
    if (document.querySelector(".settings-nav")) document.querySelector(".settings-nav .settings-back")?.click();
    await sleep(100);
    openSession("Check alpha");
    await until("the workspace", () => document.querySelector(".pane")).catch(() => {});
    document.querySelector('.layout-switch button[aria-label="One pane"]')?.click();
    await sleep(200);
  }
});

await step("no control or heading is clipped in German or Dutch", async () => {
  // GUI-048: German labels run about a third longer than English ones. In Deutsch and
  // Nederlands the sidebar, a session with its composer controls, every Settings page and
  // the usage pane are walked, and a control or heading whose words do not fit its box
  // fails. What comes from a user, an agent or a file (a session's title, a path, a
  // message) may be cut short on purpose, so only the app's own controls are measured.
  window.appcheckNotReloaded = true;
  const pick = async (value) => {
    document.querySelector(".settings-link").click();
    (await until("the Appearance page", () => withText(".settings-nav button", "Appearance") ?? withText(".settings-nav button", "Darstellung") ?? withText(".settings-nav button", "Weergave"))).click();
    const select = await until("the language", () => document.querySelector(".settings-page select.language-select"));
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(select, value);
    select.dispatchEvent(new Event("change", { bubbles: true }));
    await until(`the page in ${value}`, () => document.documentElement.lang === value, 8000);
    document.querySelector(".settings-nav .settings-back").click();
    await sleep(200);
  };
  const CONTROLS = "button, [role=tab], [role=menuitem], h1, h2, h3, h4, legend, th, summary, label, .badge";
  // Text that is not the app's own: titles, paths, names and what models wrote.
  const THEIRS = ".session-row, .row-title, .conversation, .transcript, .markdown, .tag, .git-mark, .path, code, .help-snippet, .project-name, .session-title, .file-name, .diff, .usage-row-name, .model-name, select, option";
  const clipped = (root, where) => {
    if (!root) return [`${where}: not found`];
    const out = [];
    for (const el of [root, ...root.querySelectorAll("*")]) {
      if (!el.matches(CONTROLS) && getComputedStyle(el).textOverflow !== "ellipsis") continue;
      if (el.closest(THEIRS) || !el.getClientRects().length || !el.textContent.trim()) continue;
      const style = getComputedStyle(el);
      if (style.display === "inline" || style.visibility === "hidden") continue;
      // A box that scrolls on purpose is not clipped.
      if (/auto|scroll/.test(style.overflowX + style.overflowY)) continue;
      const wide = el.scrollWidth > el.clientWidth + 1;
      const tall = style.overflowY !== "visible" && el.scrollHeight > el.clientHeight + 1;
      if (wide || tall) out.push(`${where}: ${el.tagName.toLowerCase()} "${el.textContent.trim().slice(0, 40)}" ${el.scrollWidth}x${el.scrollHeight} in ${el.clientWidth}x${el.clientHeight}`);
    }
    return out;
  };
  const found = [];
  try {
    for (const lang of ["de", "nl"]) {
      await pick(lang);
      openSession("Check alpha");
      await until("the conversation", () => document.querySelector(".conversation"));
      await sleep(200);
      found.push(...clipped(document.querySelector(".sidebar"), `${lang} sidebar`));
      found.push(...clipped(activePane(), `${lang} session`));
      document.querySelector(".settings-link").click();
      await until("Settings", () => document.querySelector(".settings-nav"));
      const pages = all(".settings-nav button:not(.settings-back)").length;
      for (let i = 0; i < pages; i++) {
        const button = all(".settings-nav button:not(.settings-back)")[i];
        button.click();
        await sleep(250);
        found.push(...clipped(document.querySelector(".settings-content"), `${lang} Settings, ${button.textContent.trim()}`));
      }
      found.push(...clipped(document.querySelector(".settings-nav"), `${lang} Settings' list`));
      document.querySelector(".settings-nav .settings-back").click();
      await sleep(200);
      openSession("Check alpha");
      await until("the workspace", () => document.querySelector(".pane"));
      document.querySelector('.layout-switch button[aria-label]:nth-child(4)')?.click();
      await sleep(300);
      if (!document.querySelector(".pane-usage")) {
        const show = { de: "Nutzung zeigen", nl: "Gebruik tonen" }[lang];
        (await until("an empty pane's Show usage", () => all(".pane-empty .row button").find((b) => b.textContent === show), 5000).catch(() => null))?.click();
      }
      const usage = await until("the usage pane", () => document.querySelector(".pane-usage"), 5000).catch(() => null);
      await sleep(300);
      found.push(...clipped(usage, `${lang} usage pane`));
      document.querySelector(".layout-switch button[aria-label]:nth-child(1)")?.click();
      await sleep(200);
    }
  } finally {
    if (document.documentElement.lang !== "en") await pick("en").catch(() => {});
    if (document.querySelector(".settings-nav")) document.querySelector(".settings-nav .settings-back")?.click();
    document.querySelector('.layout-switch button[aria-label="One pane"]')?.click();
    await sleep(200);
    openSession("Check alpha");
    await until("the workspace", () => document.querySelector(".pane")).catch(() => {});
  }
  check("no control or heading is clipped in German or Dutch", found.length === 0, `${found.length}: ${found.slice(0, 15).join("; ")}`);
  check("no reload in between", window.appcheckNotReloaded === true);
});

// The in-app demo (GUI-045): started from the palette, one scripted turn that meets each
// answer of the policy, one demo at a time, and "Remove demo" leaves nothing behind. It
// runs before the bypass step and removes what it made, so later steps find the app as
// it was.
await step("the help is a book of topics, searchable, leading into the app", async () => {
  // The sidebar's icon buttons, Help among them, are easy targets.
  const small = all(".footer-links .icon-button, .footer-row .settings-link, .layout-switch button").filter((b) => {
    const r = b.getBoundingClientRect();
    return r.width < 32 || r.height < 32;
  });
  check("the sidebar's icon buttons are at least 32 px", !small.length, small.map((b) => `${b.getAttribute("aria-label")} ${Math.round(b.getBoundingClientRect().width)}x${Math.round(b.getBoundingClientRect().height)}`).join(", "));
  named("button", "Help").click();
  const nav = await until("the help", () => document.querySelector(".help-nav"), 5000);
  const topics = all(".help-topic", nav).map((b) => b.textContent);
  check("the help lists its topics", topics.length >= 10 && topics[0] === "Getting started", topics.join(", "));
  check("it opens on the first topic", document.querySelector(".help-page h1")?.textContent === "Getting started", document.querySelector(".help-page h1")?.textContent);
  withText(".help-topic", "Permissions and the policy").click();
  await until("the permissions page", () => document.querySelector(".help-page h1")?.textContent?.startsWith("Permissions"), 4000);
  check("a topic opens on a click", true);
  const box = nav.querySelector("input");
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(box, "deny rule");
  box.dispatchEvent(new Event("input", { bubbles: true }));
  const hit = await until("a search result", () => nav.querySelector(".help-snippet"), 4000);
  check("a search quotes what it found", /deny/i.test(hit.textContent), hit.textContent);
  check("the page renders no markup from its text", !document.querySelector(".help-page script, .help-page img, .help-page iframe"));
  withText(".help-opens button", "Open Policy").click();
  await until("the Policy page", () => named("select", "Effect"), 5000);
  check("a topic leads to its page in the app", true);
  await openPalette("help permissions");
  check("the palette finds help topics", !!all('.palette-section[aria-label="Help"] .palette-item').length);
  document.querySelector(".palette input").dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
  await until("the palette to close", () => !document.querySelector(".palette"), 4000);
});

await step("an open help page follows the language", async () => {
  // GUI-048: a help page open while the language changes comes back in that language,
  // on the same topic, its words from the translated page and not English.
  window.appcheckNotReloaded = true;
  const pick = async (value) => {
    document.querySelector(".settings-link").click();
    (await until("the Appearance page", () => withText(".settings-nav button", "Appearance") ?? withText(".settings-nav button", "Darstellung") ?? withText(".settings-nav button", "Weergave"))).click();
    const select = await until("the language", () => document.querySelector(".settings-page select.language-select"));
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(select, value);
    select.dispatchEvent(new Event("change", { bubbles: true }));
    await until(`the page in ${value}`, () => document.documentElement.lang === value, 8000);
    document.querySelector(".settings-nav .settings-back").click();
    await sleep(100);
  };
  const title = () => document.querySelector(".help-page h1")?.textContent ?? "";
  const body = () => document.querySelector(".help-body")?.textContent ?? "";
  try {
    named("button", "Help").click();
    await until("the help", () => document.querySelector(".help-nav"), 5000);
    withText(".help-topic", "Permissions and the policy").click();
    await until("the permissions page", () => title() === "Permissions and the policy", 4000);
    const english = body();
    for (const [lang, heading, help] of [["de", "Berechtigungen und die Richtlinie", "Hilfe"], ["nl", "Toestemmingen en het beleid", "Help"]]) {
      await pick(lang);
      if (!document.querySelector(".help-page")) named("button", help)?.click();
      const shown = await until(`the page in ${lang}`, () => title() === heading, 8000).catch(() => false);
      check(`in ${lang} the open topic is shown in ${lang}`, shown, title());
      check(`its text is the ${lang} page, not English`, body().length > 200 && body() !== english, body().slice(0, 80));
      check(`the help's own words are ${lang}`, document.querySelector(".help-nav h2")?.textContent === help && document.querySelector(".help-nav input")?.placeholder !== "Search the help", document.querySelector(".help-nav input")?.placeholder);
    }
    await pick("en");
    if (!document.querySelector(".help-page")) named("button", "Help")?.click();
    await until("English again", () => title() === "Permissions and the policy", 8000).catch(() => {});
    check("English comes back", title() === "Permissions and the policy" && body() === english, title());
    check("no reload in between", window.appcheckNotReloaded === true);
  } finally {
    if (document.documentElement.lang !== "en") await pick("en").catch(() => {});
    openSession("Check alpha");
    await until("the conversation", () => document.querySelector(".conversation"));
  }
});

await step("suggested MCP servers are added through the usual form", async () => {
  await openPalette("MCP servers");
  (await until("the entry", () => all(".palette-item").find((i) => i.querySelector(".palette-title")?.textContent === "MCP servers"), 4000)).click();
  await until("the suggestions", () => all(".mcp-suggestion").length > 0, 8000);
  const names = all(".mcp-suggestion strong").map((e) => e.textContent);
  check("six servers are suggested", names.join(",") === "GitHub,Linear,Sentry,Notion,Context7,Playwright", names.join(","));
  check("each says where it runs", all(".mcp-suggestion").every((c) => /remote|runs locally/.test(c.textContent)));
  named("button", "Add Context7").click();
  const url = await until("the form", () => withText("label", "URL")?.querySelector("input"), 4000);
  check("the form is filled in", url.value === "https://mcp.context7.com/mcp", url.value);
  document.querySelector("form button.primary[type=submit]").click();
  const row = await until("the server", () => all(".mcp-server").find((r) => r.textContent.includes("Context7")), 8000);
  check("it waits for approval like any server", /needs approval/.test(row.textContent), row.textContent.slice(0, 80));
  await until("the suggestion marked", () => named("button", "Context7 is added"), 4000);
  check("the suggestion shows as added", named("button", "Context7 is added").disabled);
  // Leave nothing behind: a served build has no dialogs, so this answers that one.
  const real = window.fetch;
  window.fetch = (u, o) =>
    decodeURIComponent(`${u}${o?.body ?? ""}`).includes("Delete Context7?")
      ? Promise.resolve(new Response(JSON.stringify("OK"), { status: 200, headers: { "Content-Type": "application/json" } }))
      : real(u, o);
  try {
    withText("button", "Delete", row).click();
    await until("the server gone", () => !all(".mcp-server").some((r) => r.textContent.includes("Context7")), 8000);
  } finally {
    window.fetch = real;
  }
});

await step("the palette is short, grouped and one line a result", async () => {
  const open = async (query) => {
    if (!document.querySelector(".palette")) window.dispatchEvent(new KeyboardEvent("keydown", { key: "k", metaKey: true, bubbles: true }));
    const box = await until("the palette", () => document.querySelector(".palette input"), 4000);
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(box, query);
    box.dispatchEvent(new Event("input", { bubbles: true }));
    await sleep(100);
    return box;
  };
  const headings = () => all(".palette-heading").map((h) => h.textContent);
  const box = await open("");
  const radius = parseFloat(getComputedStyle(document.querySelector(".palette")).borderTopLeftRadius);
  check("the palette is a panel, not a pill", radius > 0 && radius <= 20, `${radius}px`);
  const rest = headings();
  check("before typing it lists only the short sections", rest.length > 0 && rest.every((h) => ["Waiting for you", "Actions", "Recent sessions", "Go to"].includes(h)), rest.join(", "));
  const recent = all('.palette-section[aria-label="Recent sessions"] .palette-item').length;
  check("at most five recent sessions", recent > 0 && recent <= 5, `${recent}`);
  const tall = all(".palette-item").filter((i) => i.getBoundingClientRect().height > 40);
  check("every result is one line", tall.length === 0, tall.map((i) => i.textContent).slice(0, 3).join(" | "));
  await open("/");
  const slash = headings();
  check("a slash lists only the agent's commands", slash.every((h) => h === "Agent commands"), slash.join(", "));
  await open("settings");
  const found = headings();
  check("a search groups what it finds", found.length > 0 && all(".palette-section").every((sec) => sec.querySelectorAll(".palette-item").length <= 6), found.join(", "));
  check("the best match comes first", (document.querySelector(".palette-item .palette-title")?.textContent ?? "") === "Settings", document.querySelector(".palette-item")?.textContent);
  box.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
  await until("the palette to close", () => !document.querySelector(".palette"), 4000);
});

await step("the demo runs a tour and removes itself", async () => {
  const TITLE = "Demo: your policy at work";
  const demoRows = () => all(".session-row").filter((r) => r.textContent.includes(TITLE));
  const demoGroups = () => all(".project-name").filter((h) => (h.title ?? "").endsWith("-demo"));
  const palette = async (query) => {
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "k", metaKey: true, bubbles: true }));
    const box = await until("the palette", () => document.querySelector(".palette input"), 4000);
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(box, query);
    box.dispatchEvent(new Event("input", { bubbles: true }));
    return box;
  };
  const runDemo = async () => {
    await palette("Try the demo");
    (await until("the palette entry", () => withText(".palette-item", "Try the demo"), 4000)).click();
    await until("the palette to close", () => !document.querySelector(".palette"), 4000);
  };
  const projectEntries = async () => {
    const box = await palette("new session demo");
    await sleep(100);
    const found = all('.palette-section[aria-label="New session in"] .palette-item').filter((i) => (i.querySelector(".palette-detail")?.textContent ?? "").endsWith("-demo"));
    box.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
    await until("the palette to close", () => !document.querySelector(".palette"), 4000);
    return found.length;
  };

  check("no demo to begin with", demoRows().length === 0 && demoGroups().length === 0);
  await runDemo();
  const banner = await until("the demo banner", () => withText(".demo-banner", "This is the demo: a scripted agent in a scratch project."), 20000);
  check("the demo says what it is", banner.textContent.includes("Nothing outside it is touched.") && withText("button", "Remove demo", banner));
  await until("the demo's title", () => activePane().querySelector(".session-titleline h2")?.textContent === TITLE, 5000);
  check("the demo is one session in the sidebar", demoRows().length === 1, `${demoRows().length} rows`);
  check("the demo has a project of its own", demoGroups().length === 1, `${demoGroups().length} groups`);

  // The tour asks about notes.md and .env; the rest the policy answers itself.
  const pane = activePane();
  const asked = [];
  await until("the closing message", () => {
    for (const b of all(".approval-buttons button", pane)) {
      if (b.textContent.includes("Allow once")) {
        asked.push(b.closest(".approval").textContent);
        b.click();
      }
    }
    return withText(".msg-agent, .md", "That is the tour", pane);
  }, 45000);
  check("the tour asks twice", asked.length === 2, `${asked.length} requests`);
  check("it asks about notes.md and .env", asked.some((t) => t.includes("notes.md")) && asked.some((t) => t.includes(".env")), asked.map((t) => t.slice(0, 60)).join(" | "));
  const refused = all(".conversation .decision-denied", pane).map((d) => d.textContent.trim());
  check("the git push is refused by the rule", refused.includes("denied by rule"), refused.join(", "));
  check("the hook is refused as a protected path", refused.includes("denied by protected_path"), refused.join(", "));
  check("nothing else is refused", refused.length === 2, refused.join(", "));

  // A second start opens the same session.
  await runDemo();
  await sleep(500);
  check("a second start opens the same demo", demoRows().length === 1 && !!withText(".demo-banner", "This is the demo"), `${demoRows().length} rows`);
  check("the demo project is listed", (await projectEntries()) === 1);

  // Removing asks first, then deletes the session and the project. A served build has no
  // native dialogs, so this answers that call.
  let answer = "Cancel";
  let asks = 0;
  const real = window.fetch;
  window.fetch = (u, o) => {
    if (decodeURIComponent(`${u}${o?.body ?? ""}`).includes("Remove the demo?")) {
      asks++;
      return Promise.resolve(new Response(JSON.stringify(answer), { status: 200, headers: { "Content-Type": "application/json" } }));
    }
    return real(u, o);
  };
  try {
    withText(".demo-banner button", "Remove demo").click();
    await until("the question", () => asks === 1, 4000);
    await sleep(500);
    check("removing asks first", asks === 1, `asked ${asks} times`);
    check("declining keeps the demo", demoRows().length === 1);
    answer = "OK";
    withText(".demo-banner button", "Remove demo").click();
    await until("the demo to go", () => demoRows().length === 0, 8000);
    check("removing takes the session out of the sidebar", true);
    await until("the project to go", () => demoGroups().length === 0, 5000);
    check("removing takes the project out of the sidebar", true);
    // The list is read again after the removal answers, so it may take a moment.
    let left = await projectEntries();
    for (let i = 0; i < 20 && left !== 0; i++) {
      await sleep(250);
      left = await projectEntries();
    }
    check("the demo project is not listed", left === 0, `${left} entries`);
  } finally {
    window.fetch = real;
  }
});

// WFL-001 to WFL-006: the built-in workflow from end to end on the demo provider:
// install its skills, slice into two issues (the second blocked by the first), see
// them on the Board, run them AFK, and QA what the run finished.
await step("a workflow slices a brief and runs its issues AFK", async () => {
  const setSelect = (el, value) => {
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(el, value);
    el.dispatchEvent(new Event("change", { bubbles: true }));
  };
  // Settings → Workflows: the built-in one, and its skills installed on request.
  document.querySelector(".settings-link").click();
  (await until("the Workflows page", () => withText(".settings-nav button", "Workflows"))).click();
  const card = await until("the built-in workflow", () => all(".workflow-card").find((c) => c.querySelector("h3")?.textContent === "Plan, slice, run AFK"), 8000);
  check("the built-in workflow lists its stages", ["/grill", "/prd", "/slice", "/afk", "/review", "/cleanup"].every((c) => card.textContent.includes(c)), card.textContent.slice(0, 120));
  check("its skills wait to be installed", card.textContent.includes("not installed"));
  withText("button", "Install skills", card).click();
  await until("the skills installed", () => card.textContent.includes("Installed grill-the-brief"), 8000);
  check("installing names what it wrote", !card.textContent.includes("not installed"), card.textContent.slice(0, 200));
  // WFL-013: a model set, made here and given to the built-in workflow.
  const sets = document.querySelector('.model-sets[aria-label="Model sets"]');
  check("Settings → Workflows has model sets", !!sets && sets.textContent.includes("No model sets yet"), sets?.textContent.slice(0, 120));
  withText("button", "New model set", sets).click();
  const editor = await until("the set's editor", () => sets.querySelector(".model-set-editor"), 4000);
  const typeIn = (el, text) => {
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(el, text);
    el.dispatchEvent(new Event("input", { bubbles: true }));
  };
  typeIn(all("label", editor).find((l) => l.textContent.startsWith("Name")).querySelector("input"), "Big plans, small works");
  const rowModel = (legend) => all(".model-set-row", editor).find((f) => f.querySelector("legend")?.textContent === legend)?.querySelector("select");
  await until("the models", () => [...(rowModel("Issues")?.options ?? [])].some((o) => o.value === "demo-small\nsmall"), 8000);
  setSelect(rowModel("Planning stages"), "demo-small\nsmall");
  setSelect(rowModel("Issues"), "demo-small\nsmall");
  await sleep(100);
  const rowEffort = await until("the issues row's efforts", () => {
    const el = all(".model-set-row", editor).find((f) => f.querySelector("legend")?.textContent === "Issues")?.querySelectorAll("select")[1];
    return el && !el.disabled && el;
  }, 8000);
  setSelect(rowEffort, "low");
  await sleep(100);
  withText("button", "Save model set", editor).click();
  const saved = await until("the set listed", () => sets.querySelector('li[aria-label="Big plans, small works"]'), 8000);
  check("the set lists its rows", /Planning stages\s*small/.test(saved.textContent) && /Issues\s*small · low/.test(saved.textContent) && /Reviews\s*Not fixed/.test(saved.textContent), saved.textContent);
  setSelect(all("label", card).find((l) => l.textContent.startsWith("Model set")).querySelector("select"), "big-plans-small-works");
  await sleep(300);
  document.querySelector(".settings-nav .settings-back").click();
  await sleep(100);
  document.querySelector(".settings-link").click();
  (await until("the Workflows page again", () => withText(".settings-nav button", "Workflows"))).click();
  const again = await until("the built-in workflow again", () => all(".workflow-card").find((c) => c.querySelector("h3")?.textContent === "Plan, slice, run AFK"), 8000);
  const chosen = await until("its set", () => all("label", again).find((l) => l.textContent.startsWith("Model set"))?.querySelector("select"), 4000);
  check("the workflow keeps its model set", chosen.value === "big-plans-small-works", chosen.value);
  document.querySelector(".settings-nav .settings-back").click();
  await sleep(100);
  // WFL-014: a workflow chosen in the new-session form brings its planning model, and says so.
  named(".sidebar header button", "New session").click();
  const field = (name) => [...document.querySelectorAll("label")].find((l) => l.firstChild?.textContent?.trim() === name);
  const wfPick = await until("the form's workflow", () => field("Workflow")?.querySelector("select"), 8000);
  await until("the workflows listed", () => [...wfPick.options].some((o) => o.value === "plan-slice-afk"), 8000);
  setSelect(wfPick, "plan-slice-afk");
  const fromSet = await until("the planning model", () => field("Provider")?.querySelector("select")?.value === "demo-small" && field("Model")?.querySelector("input")?.value === "small" && document.querySelector(".model-from-set"), 8000).catch(() => null);
  check("the form starts a workflow's session on its planning model, and says from where", fromSet?.textContent === "From the model set Big plans, small works, at effort: The profile's", fromSet?.textContent ?? `${field("Provider")?.querySelector("select")?.value} ${field("Model")?.querySelector("input")?.value}`);

  // A trusted project, a session in it with the workflow, and /slice.
  openSession("Check workflow");
  await until("the conversation", () => activePane().querySelector(".conversation"));
  const trust = await until("the trust button", () => withText("button", "Trust project", activePane()), 8000);
  trust.click();
  await until("trusted", () => !withText("button", "Trust project", activePane()), 8000);
  const picker = await until("the workflow picker", () => all(".session-control", activePane()).find((l) => l.textContent.includes("Workflow"))?.querySelector("select"), 8000);
  await until("the workflow listed", () => [...picker.options].some((o) => o.value === "plan-slice-afk"), 8000);
  setSelect(picker, "plan-slice-afk");
  await until("the workflow chosen", () => picker.value === "plan-slice-afk", 8000);
  // WFL-008: choosing it shows the stage strip, with the first stage next.
  const strip = await until("the stage strip", () => activePane().querySelector(".workflow-strip"), 8000);
  const steps = () => all(".workflow-step", strip).map((s) => [...s.classList].find((c) => /^workflow-step-(done|running|next|todo)$/.test(c)).replace("workflow-step-", "") + ":" + s.querySelector(".workflow-step-text")?.firstChild?.textContent);
  check("the strip shows the path, the first stage next", steps().join("|") === "next:Grill the brief|todo:Write the PRD|todo:Slice into issues|todo:Run the AFK issues", steps().join("|"));
  check("its Next button names the first stage", withText(".workflow-next", "Next: Grill the brief", strip));
  const sr = strip.getBoundingClientRect(), cr = activePane().querySelector(".composer").getBoundingClientRect();
  check("the strip fits above the composer", sr.left >= cr.left - 1 && sr.right <= cr.right + 1 && sr.bottom <= cr.top + 1 && sr.height < 64 && strip.scrollWidth <= strip.clientWidth + 1, JSON.stringify({ strip: [sr.left, sr.right, sr.height, strip.scrollWidth, strip.clientWidth], composer: [cr.left, cr.right, cr.top] }));
  // In a narrow pane the stages shrink to dots and nothing lies on anything else (the
  // strip's parts, as they are shown, never overlap, and none leaves the strip).
  const parts = () => [...strip.querySelectorAll(".workflow-step, .workflow-wrote, .workflow-strip-actions > button")].filter((e) => e.getBoundingClientRect().width > 0);
  const tangled = () => {
    const r = parts().map((e) => [e.className.split(" ")[0], e.getBoundingClientRect()]);
    const s = strip.getBoundingClientRect();
    const out = [];
    r.forEach(([n, a], i) => {
      if (a.left < s.left - 1 || a.right > s.right + 1) out.push(`${n} leaves the strip`);
      r.slice(i + 1).forEach(([m, b]) => {
        if (a.left < b.right - 1 && b.left < a.right - 1 && a.top < b.bottom - 1 && b.top < a.bottom - 1) out.push(`${n} lies on ${m}`);
      });
    });
    return out;
  };
  const narrow = async (label) => {
    const session = strip.closest(".session");
    for (const width of ["380px", "300px"]) {
      session.style.maxWidth = width;
      await sleep(150);
      const dots = all(".workflow-step:not(.workflow-step-current)", strip).filter((e) => e.getBoundingClientRect().width > 0);
      const current = strip.querySelector(".workflow-step-current")?.getBoundingClientRect();
      check(`${label}: nothing in the strip overlaps at ${width}`, tangled().length === 0 && strip.getBoundingClientRect().height < 90, tangled().join("; ") + " " + strip.getBoundingClientRect().height);
      check(`${label}: the other stages are dots and the current one is named at ${width}`, dots.every((e) => e.getBoundingClientRect().width <= 14) && (!current || current.width > 40), JSON.stringify({ dots: dots.map((e) => Math.round(e.getBoundingClientRect().width)), current: current && Math.round(current.width) }));
      if (strip.querySelector(".workflow-next")) check(`${label}: the Next button stays in view at ${width}`, strip.querySelector(".workflow-next").getBoundingClientRect().right <= strip.getBoundingClientRect().right + 1);
    }
    session.style.maxWidth = "";
    await sleep(150);
  };
  await narrow("first stage next");
  check("a wide strip keeps its stages as named pills", all(".workflow-step", strip).every((e) => e.getBoundingClientRect().width > 40) && tangled().length === 0, tangled().join("; "));
  const box = activePane().querySelector(".composer textarea");
  box.focus();
  type(box, "/");
  await sleep(150);
  const listed = all('.mention-picker[aria-label="Commands"] li', activePane()).map((l) => l.textContent);
  check("its stages are slash commands", ["grill", "prd", "slice", "afk"].every((c) => listed.some((l) => l.includes(c))), listed.join(" | ").slice(0, 200));
  const ended = () => all(".turn-stop", activePane()).length;
  const before = ended();
  type(box, "/slice");
  await sleep(100);
  (await until("Send", () => { const b = named(".composer-actions button.primary", "Send", activePane()); return b && !b.disabled && b; })).click();
  await until("the slice to end", () => {
    const allow = activePane().querySelector(".approval-buttons button.primary");
    if (allow) allow.click();
    return ended() > before;
  }, 45000);
  const sent = await until("the stage's prompt", () => all(".msg-user").find((m) => m.textContent.includes("Workflow stage: slice")), 5000).catch(() => null);
  check("the stage's prompt was sent", !!sent, all(".msg-user").map((m) => m.textContent.slice(0, 60)).join(" | "));
  // WFL-014: the workflow's model set plans on Demo small, so the stage switched to it first.
  check("the stage switched the session to the set's planning model", activePane().textContent.includes("Switched to small for this stage (model set Big plans, small works)."), activePane().querySelector(".session-sub")?.textContent);
  const writes = all(".tool.tool-completed", activePane()).filter((t) => t.querySelector("summary")?.textContent.includes("write_file"));
  check("a file written has a pen beside it", writes.length > 0 && writes.every((t) => t.querySelector('.edit-mark[aria-label="Edited a file"] svg')), `${writes.length} writes`);

  // WFL-008: the conversation marks where the stage began, and the strip moves on,
  // saying what the stage wrote.
  check("the conversation marks the stage", withText(".workflow-stage-marker", "Workflow stage: Slice into issues", activePane()));
  await until("the strip to move on", () => steps().includes("done:Slice into issues"), 8000);
  check("after slicing, the AFK run is next", steps().includes("next:Run the AFK issues"), steps().join("|"));
  const wrote = strip.querySelector(".workflow-wrote")?.textContent ?? "";
  check("the strip says what the stage wrote", wrote.includes("001-first-slice.md"), wrote);
  await narrow("with what it wrote");

  // The run: its Next button opens the dialog with what it will want, allowed up
  // front, then both issues are done and reviewed.
  withText(".workflow-next", "Next: Run the AFK issues", strip).click();
  const dialog = await until("the run's dialog", () => document.querySelector(".afk-dialog"), 8000);
  await until("the plan", () => dialog.textContent.includes("First:"), 8000);
  check("the dialog says what it does first", dialog.textContent.includes("First slice"), dialog.textContent.slice(0, 160));
  check("the dialog offers the run's commands", dialog.textContent.includes("git commit *"), dialog.textContent.slice(0, 300));
  const queuePaused = dialog.textContent.includes("The queue is paused");
  const afkProfile = all("label", dialog).find((l) => l.textContent.startsWith("Profile for the issues"))?.querySelector("select");
  const reviewProfile = all("label", dialog).find((l) => l.textContent.startsWith("Profile for the reviews"))?.querySelector("select");
  setSelect(afkProfile, "afk-demo");
  if (reviewProfile) setSelect(reviewProfile, "afk-demo");
  // WFL-010, WFL-011: the issues can go to another model than the profile's, on any
  // provider that can be called unasked, here the second demo provider.
  const smallModel = async (d) => {
    const sel = await until("the issues' models", () => {
      const el = all("label", d).find((l) => l.textContent.startsWith("Model for the issues"))?.querySelector("select");
      return el && el.options.length > 1 && el;
    }, 8000);
    const small = [...sel.querySelectorAll('optgroup[label="Demo small"] option')].find((o) => o.value === "demo-small\nsmall");
    if (small) setSelect(sel, small.value);
    // WFL-012: Demo small's models take an effort; the run's issues go at low.
    const effort = await until("the issues' efforts", () => {
      const el = all("label", d).find((l) => l.textContent.startsWith("Effort for the issues"))?.querySelector("select");
      return el && !el.disabled && [...el.options].some((o) => o.value === "low") && el;
    }, 8000).catch(() => null);
    if (effort) setSelect(effort, "low");
    return sel;
  };
  // WFL-014: the workflow's model set fills the issues' model and effort.
  const setNote = await until("the set's model", () => {
    const l = all("label", dialog).find((x) => x.textContent.startsWith("Model for the issues"));
    return l?.querySelector("select")?.value === "demo-small\nsmall" && l.querySelector(".afk-model-source");
  }, 8000).catch(() => null);
  check("the dialog starts from the workflow's model set", setNote?.textContent === "from model set Big plans, small works", setNote?.textContent);
  const issuesModel = await smallModel(dialog);
  check("the run offers the profile's models", [...issuesModel.options].some((o) => o.value.endsWith("\nscripted")), [...issuesModel.options].map((o) => o.textContent).join(", "));
  check("and every callable provider's, grouped, with one to name", !!issuesModel.querySelector('optgroup[label="Demo small"]') && [...issuesModel.options].some((o) => o.textContent === "Other model on Demo small…"), all("optgroup", issuesModel).map((g) => g.label).join(", "));
  check("the issues go to Demo small", issuesModel.value === "demo-small\nsmall", issuesModel.value);
  const issuesEffort = all("label", dialog).find((l) => l.textContent.startsWith("Effort for the issues"))?.querySelector("select");
  check("Demo small's model offers its efforts, low chosen", issuesEffort?.value === "low", [...(issuesEffort?.options ?? [])].map((o) => o.value).join(","));
  // WFL-009: a command of the user's own joins the list, ticked.
  const another = dialog.querySelector('input[aria-label="Another command to allow"]');
  Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set.call(another, "npm run e2e");
  another.dispatchEvent(new Event("input", { bubbles: true }));
  await sleep(50);
  withText("button", "Add", dialog).click();
  await until("the command added", () => all(".afk-commands label", dialog).some((l) => l.textContent.includes("npm run e2e") && l.textContent.includes("yours")), 4000);
  check("a command of the user's own can be allowed", true);
  await sleep(100);
  (await until("Start", () => { const b = withText("button", "Start the run", dialog); return b && !b.disabled && b; }, 4000)).click();
  await until("the dialog closed", () => !document.querySelector(".afk-dialog"), 8000);

  // Starting shows the Board: the project's row, with the run's first issue in Working
  // and the second, which waits on it, in Backlog (GUI-016).
  const laneEl = () => document.querySelector('.board-lane[aria-label="Backlog of workflow"]');
  await until("the project's row", laneEl, 15000);
  const cards = () => all(".backlog-card", laneEl() ?? document);
  const issueCard = (n) => cards().find((c) => c.getAttribute("aria-label")?.startsWith(`${n}:`));
  const column = (c) => c?.closest(".board-col")?.getAttribute("aria-label");
  await until("two issues", () => cards().length === 2, 10000);
  check("the row has the four columns", all(".board-col h3", laneEl()).map((h) => h.firstChild.textContent.trim()).join("|") === "Backlog|Working|In review|Done", all(".board-col h3", laneEl()).map((h) => h.textContent).join("|"));
  // With the queue paused (the step before leaves it so) the issue waits in Working until
  // it is resumed; with it running, a quick issue can already be in review when the Board
  // next looks, so either column shows the run has taken it up.
  await until("the first issue taken up", () => column(issueCard("001")) !== "Backlog", 15000);
  check("the first issue is taken up by the run, an agent's", (queuePaused ? column(issueCard("001")) === "Working" : ["Working", "In review"].includes(column(issueCard("001")))) && issueCard("001").textContent.includes("AFK"), `${column(issueCard("001"))}: ${issueCard("001")?.textContent}`);
  // Only while the queue holds the run can the second still be waiting for the first.
  if (queuePaused) check("the second waits on the first in Backlog", column(issueCard("002")) === "Backlog" && issueCard("002").textContent.includes("after 001"), issueCard("002")?.textContent);
  // A card is compact: a title of at most two lines, the file only on hover.
  check("a card holds the title, one meta line and no file path", issueCard("001").querySelector(".issue-title")?.textContent === "First slice" && !issueCard("001").textContent.includes("backlog/issues") && issueCard("001").title.includes("backlog/issues/001-first-slice.md"), issueCard("001")?.title);
  check("a title is cut after two lines", getComputedStyle(issueCard("001").querySelector(".issue-title")).webkitLineClamp === "2");
  // The queue step above left the queue paused, which holds the run's issues: the
  // dialog said so, and the Queue page resumes it.
  if (queuePaused) {
    // Starting shows the Board, where the run and its first issue say why nothing moves.
    // The run learns why on the schedule loop's next pass after queueing the issue, which
    // can be its 30-second tick.
    const waiting = await until("the run waiting", () => document.querySelector('[aria-label="Backlog of workflow"] .backlog-run-waiting')?.textContent, 35000);
    check("the run says its issue waits for the paused queue", waiting.includes("001-first-slice waiting to start: the queue is paused"), waiting);
    const firstCard = issueCard("001");
    check("the issue's card says so too", firstCard?.querySelector(".backlog-step-waiting")?.textContent.includes("AFK run: waiting to start"), firstCard?.textContent);
    const queueIcon = all(".footer-links button").find((b) => /^Queue\b/.test(b.getAttribute("aria-label") ?? ""));
    // The icon looks at the queue every few seconds.
    await until("the icon's count", () => /waiting/.test(queueIcon?.getAttribute("aria-label") ?? ""), 12000).catch(() => {});
    check("the Queue icon counts the run's issue waiting in the paused queue", /waiting/.test(queueIcon?.getAttribute("aria-label") ?? ""), queueIcon?.getAttribute("aria-label"));
    // Stopping the run while its issue waits for the queue puts the issue back in Backlog
    // with a red mark saying why (GUI-016); a new run that takes it up clears the mark.
    withText("button", "Stop", laneEl()).click();
    const back = await until("the stopped issue back in Backlog", () => column(issueCard("001")) === "Backlog" && issueCard("001").querySelector(".issue-mark-stopped") && issueCard("001"), 20000);
    check("a stopped issue is back in Backlog with a red mark saying why", /The run stopped here: /.test(back.querySelector(".issue-mark-stopped").textContent) && getComputedStyle(back.querySelector(".issue-mark-stopped")).color !== getComputedStyle(back).color, back.querySelector(".issue-mark-stopped").textContent);
    withText("button", "Run AFK issues…", laneEl()).click();
    const again = await until("the run's dialog again", () => document.querySelector(".afk-dialog"), 8000);
    await until("the plan again", () => again.textContent.includes("First:"), 8000);
    for (const label of ["Profile for the issues", "Profile for the reviews"]) {
      const select = all("label", again).find((l) => l.textContent.startsWith(label))?.querySelector("select");
      if (select) setSelect(select, "afk-demo");
    }
    await smallModel(again);
    await sleep(150);
    (await until("Start again", () => { const b = withText("button", "Start the run", again); return b && !b.disabled && b; }, 4000)).click();
    await until("the dialog closed again", () => !document.querySelector(".afk-dialog"), 8000);
    await until("a newer run takes the issue up", () => column(issueCard("001")) === "Working" && !issueCard("001").querySelector(".issue-mark-stopped"), 40000);
    check("a newer run takes the issue up and the mark is gone", column(issueCard("001")) === "Working" && !issueCard("001").querySelector(".issue-mark-stopped"));
    queueIcon.click();
    const page = await until("the queue page", () => [...document.querySelectorAll(".page h2")].find((h) => h.textContent === "Queue")?.closest(".page"), 8000);
    (await until("Resume", () => withText("button", "Resume", page), 4000)).click();
    await until("resumed", () => !page.querySelector('[role="status"]')?.textContent.startsWith("Paused"), 5000);
    named(".footer-links button", "Board").click();
  }
  // The first issue's session asks the user a question (its issue says to), so its card
  // waits for the user with the request on it, and is answered there (GUI-016).
  const needs = await until("the first issue waiting for the user", () => issueCard("001")?.classList.contains("issue-needs") && issueCard("001"), 60000);
  check("a waiting issue stays in Working with a strong mark", column(needs) === "Working" && needs.textContent.includes("Waiting for you"), `${column(needs)}: ${needs.textContent.slice(0, 90)}`);
  const headerCount = document.querySelector(".board-waiting")?.textContent ?? "";
  check("the Board says how many issues wait for the user", headerCount === "1 issue waiting for you", headerCount);
  check("the request is on the card, with its buttons and a way to its session", !!withText("button", "Skip", needs) && !!withText("button", "Open its session", needs), needs.textContent.slice(0, 160));
  check("the second issue does not wait", !issueCard("002")?.classList.contains("issue-needs"));
  // A row collapsed while one of its issues waits for the user stays open, and says why.
  const toggle = () => named("button", "Collapse workflow", laneEl()) ?? named("button", "Expand workflow", laneEl());
  toggle().click();
  await until("the note", () => laneEl()?.querySelector(".board-lane-note"), 4000);
  check("a collapsed row with an issue waiting for the user stays open and says why", !!issueCard("001") && /waits for you/.test(laneEl().querySelector(".board-lane-note").textContent) && toggle().disabled, laneEl()?.querySelector(".board-lane-note")?.textContent);
  withText("button", "Skip", issueCard("001")).click();
  await until("the card no longer waiting", () => !issueCard("001")?.classList.contains("issue-needs") && !document.querySelector(".board-waiting"), 30000);
  check("answering on the card takes the mark and the count away", !document.querySelector(".board-waiting"));
  // Nothing waits now, so the row is folded as the user left it: open it again.
  await until("the toggle usable", () => toggle() && !toggle().disabled, 8000);
  if (named("button", "Expand workflow", laneEl())) toggle().click();
  await until("the row open again", () => all(".board-col", laneEl()).length === 4, 4000);
  const line = await until("the run to finish", () => {
    const l = document.querySelector('[aria-label="Backlog of workflow"] .backlog-run-line')?.textContent;
    return l && !l.includes("going on") && l;
  }, 150000);
  check("the run did both issues", line.startsWith("AFK run finished: 2 of 2 issues done"), line);
  check("the run's line says where its issues went", line.includes("Issues on small (Demo small) · low"), line);
  // WFL-015: what it cost, by part and model; the demo's models have no price, which it says.
  // The row says the total; its tooltip, the parts and their models.
  const costEl = await until("the run's cost", () => document.querySelector('[aria-label="Backlog of workflow"] .backlog-run-cost'), 8000).catch(() => null);
  const costLine = costEl?.title ?? "";
  check("the run says what it cost, unpriced as such", costEl?.textContent === "cost: unpriced", costEl?.textContent);
  check("and what its issues and reviews cost, by model", /Issues unpriced on small/.test(costLine) && /Reviews unpriced on /.test(costLine) && /total unpriced$/.test(costLine) && !costLine.includes("$0"), costLine);
  // And the Usage page groups by workflow run.
  named(".footer-links button", "Usage").click();
  (await until("the tokens tab", () => document.querySelector("#usage-tab-tokens"), 8000)).click();
  const groupSel = await until("the grouping", () => all(".usage-filters label").find((l) => l.textContent.includes("Group by"))?.querySelector("select"), 8000);
  check("the Usage page offers By workflow run", [...groupSel.options].some((o) => o.value === "run" && o.textContent === "Workflow run"), [...groupSel.options].map((o) => o.textContent).join(", "));
  setSelect(groupSel, "run");
  const runRow = await until("the run's row", () => all(".usage-page table tbody tr td:first-child").find((td) => /^workflow · \d{4}-\d\d-\d\d \d\d:\d\d$/.test(td.textContent.trim())), 8000).catch(() => null);
  check("the demo run has its row", !!runRow, all(".usage-page table tbody tr td:first-child").map((td) => td.textContent).join(" | "));
  // GUI-051: what each of the run's commits cost; the demo's models have no price, so in tokens.
  (await until("the efficiency tab", () => document.querySelector("#usage-tab-efficiency"), 8000)).click();
  const work = await until("what a commit costs", () => { const w = document.querySelector(".work-cost"); return w?.querySelector(".work-table tbody tr") && w; }, 20000).catch(() => null);
  const counted = work?.querySelector(".work-table tbody tr td:nth-child(2)")?.textContent;
  check("the run's commits are counted, in tokens as the demo has no price", Number(counted) >= 2 && /Median tokens per commit/.test(work?.textContent ?? ""), work?.textContent.slice(0, 240));
  work?.querySelector(".work-table tbody tr td button.link")?.click();
  const weekCommits = await until("the week's commits", () => all(".work-commits li", work).length >= 2, 4000).catch(() => false);
  check("a week lists its commits, each opening its session", !!weekCommits && all(".work-commits li button.link", work).length >= 2, work?.querySelector(".work-commits")?.textContent);
  named(".footer-links button", "Board").click();
  await until("the Board again", laneEl, 8000);
  const afkRow = all(".session-row").find((r) => /AFK 001/.test(r.textContent));
  check("the issue's session ran on Demo small", /^Provider: Demo small/.test(afkRow?.querySelector(".source-mark")?.getAttribute("aria-label") ?? ""), afkRow?.querySelector(".source-mark")?.getAttribute("aria-label"));
  afkRow.click();
  const lowered = await until("the effort in its session", () => /AFK 001/.test(activePane().querySelector(".session-titleline h2")?.textContent ?? "") && activePane().textContent.includes("Effort set to low"), 8000).catch(() => null);
  check("the issue's session ran at the effort chosen for the run", !!lowered, activePane().querySelector(".session-titleline h2")?.textContent);
  named(".footer-links button", "Board").click();
  await until("the Board again", laneEl, 8000);
  // (Needs attention also counts the issues now ready for QA, so ask the sessions' own dots.)
  const needing = all(".session-row").filter((r) => r.querySelector(".status-dot")?.getAttribute("aria-label") === "Needs you");
  check("no unattended session waited for an approval", needing.length === 0, needing.map((r) => r.textContent.slice(0, 40)).join(" | "));
  // Both issues are done and reviewed, and wait for the user's QA: In review.
  await until("both issues in review", () => column(issueCard("001")) === "In review" && column(issueCard("002")) === "In review", 15000);
  check("the finished issues wait in review", column(issueCard("001")) === "In review" && column(issueCard("002")) === "In review", `${column(issueCard("001"))}, ${column(issueCard("002"))}`);
  // Clicking a card opens a read-only panel: the issue's text, its file, what it waits on, and the
  // sessions that did it and reviewed it, each opening that session (GUI-016).
  issueCard("002").querySelector(".issue-title").click();
  const panel = await until("the issue's panel", () => document.querySelector(".issue-panel"), 8000);
  await until("the panel's text", () => panel.querySelector(".issue-panel-text")?.textContent.includes("Second slice, end to end."), 8000);
  check("the panel has the issue's text, its file and what it waits on", panel.querySelector("h3")?.textContent === "Second slice" && panel.textContent.includes("backlog/issues/002-second-slice.md") && /Waits on: 001/.test(panel.textContent), panel.textContent.slice(0, 160));
  check("the panel says what the issue cost, and its commit", /^Cost unpriced on .+ · commit [0-9a-f]{7}$/.test(panel.querySelector(".issue-panel-cost")?.textContent ?? ""), panel.querySelector(".issue-panel-cost")?.textContent);
  check("the panel lists the session and the review, each opening its session", all(".issue-panel-sessions li", panel).length === 2 && /AFK 002/.test(panel.textContent) && /Review: 002/.test(panel.textContent) && all(".issue-panel-sessions button", panel).length === 2, all(".issue-panel-sessions li", panel).map((l) => l.textContent).join(" | "));
  check("the panel changes nothing: its only buttons close it and open sessions", all("button", panel).every((b) => b.getAttribute("aria-label") === "Close" || b.closest(".issue-panel-sessions")), all("button", panel).map((b) => b.textContent || b.getAttribute("aria-label")).join(" | "));
  check("the keyboard reaches the panel first", document.activeElement === panel.querySelector('button[aria-label="Close"]'), document.activeElement?.outerHTML.slice(0, 80));
  document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
  await until("the panel closed by Escape", () => !document.querySelector(".issue-panel"), 4000);
  check("Escape closes the panel", !document.querySelector(".issue-panel"));
  issueCard("001").querySelector(".issue-title").click();
  await until("the panel again", () => document.querySelector(".issue-panel h3")?.textContent === "First slice", 8000);
  document.querySelector('.issue-panel button[aria-label="Close"]').click();
  await until("the panel closed by its button", () => !document.querySelector(".issue-panel"), 4000);
  check("the close button closes it", !document.querySelector(".issue-panel"));
  // Nothing in a card is wider than the card, and in a narrow window the row scrolls
  // sideways instead of squeezing its columns.
  const wide = cards().filter((c) => c.scrollWidth > c.clientWidth + 1 || all("*", c).some((e) => e.getBoundingClientRect().right > c.getBoundingClientRect().right + 1));
  check("no card holds anything wider than itself", wide.length === 0, wide.map((c) => c.getAttribute("aria-label")).join(", "));
  const page = document.querySelector(".board-page");
  const lanes = document.querySelector(".board-lanes");
  const rem = parseFloat(getComputedStyle(document.documentElement).fontSize);
  page.style.maxWidth = "640px";
  await sleep(150);
  const narrowest = Math.min(...all(".board-col", laneEl()).map((c) => c.getBoundingClientRect().width));
  check("in a narrow window the Board scrolls sideways", lanes.scrollWidth > lanes.clientWidth + 1, `${lanes.scrollWidth} vs ${lanes.clientWidth}`);
  check("and its columns keep their width", narrowest >= 13 * rem, `${Math.round(narrowest)} px, a rem is ${rem}`);
  check("and no card holds anything wider than itself", cards().every((c) => c.scrollWidth <= c.clientWidth + 1), "");
  page.style.maxWidth = "";

  // WFL-009: the run in full, each issue with its session and review.
  withText(".backlog-run-details", "Show the run").click();
  const view = await until("the run view", () => document.querySelector(".afk-run-view"), 8000);
  const runSteps = all(".afk-run-step", view);
  check("the run view lists both issues, ready for QA", runSteps.length === 2 && runSteps.every((s) => s.textContent.includes("ready for QA") && withText("button", "Open its session", s) && withText("button", "Open its review", s)), runSteps.map((s) => s.textContent.slice(0, 80)).join(" | "));
  withText("button", "Close", view).click();
  await until("the run view closed", () => !document.querySelector(".afk-run-view"), 4000);

  // Ready for QA, each with its review, until QA'd.
  (await until("Needs attention", () => document.querySelector(".attention-link"), 8000)).click();
  const qa = () => all(".attention-qa");
  await until("two issues ready for QA", () => qa().length === 2, 10000);
  check("each issue waits for QA with its review", qa().every((s) => s.textContent.includes("Ready for QA") && s.textContent.includes("found nothing wrong")), qa().map((s) => s.textContent.slice(0, 80)).join(" | "));
  // On the Board the same two cards say so too: "Ready for your QA", with the review's
  // answer, and the buttons to check the issue off or to open its session and its review.
  named(".footer-links button", "Board").click();
  const ready = await until("both cards ready for QA", () => issueCard("001")?.querySelector(".issue-mark-qa") && issueCard("002")?.querySelector(".issue-mark-qa") && issueCard("001"), 15000);
  check("a card ready for QA says so, with the review's answer", ready.querySelector(".issue-mark-qa").textContent === "Ready for your QA" && ready.querySelector(".issue-reply")?.textContent.includes("found nothing wrong"), ready.textContent.slice(0, 160));
  check("and has QA done, Open its session and Open the review", !!withText("button", "QA done", ready) && !!withText("button", "Open its session", ready) && !!withText("button", "Open the review", ready), all("button", ready).map((b) => b.textContent).join(" | "));
  // QA done on the card moves it to Done, takes it out of Needs attention, and leaves the other.
  withText("button", "QA done", ready).click();
  await until("the QA'd issue in Done", () => column(issueCard("001")) === "Done", 15000);
  check("the QA'd issue is Done and the other still in review", column(issueCard("001")) === "Done" && column(issueCard("002")) === "In review", `${column(issueCard("001"))}, ${column(issueCard("002"))}`);
  check("a Done card has no QA button", !withText("button", "QA done", issueCard("001")));
  (await until("Needs attention", () => document.querySelector(".attention-link"), 8000)).click();
  await until("one left for QA", () => qa().length === 1, 10000);
  check("QA done on the Board takes the issue off Needs attention", qa().length === 1 && qa()[0].textContent.includes("Second slice"), qa().map((q) => q.textContent.slice(0, 60)).join(" | "));
});

// GUI-016: a row folds away under its header and stays folded, a filter picks one project,
// and Done shows its last five with a way to all of them. The check's other project has
// seven finished issues; the workflow's row is the other row.
await step("the Board's rows collapse, a filter picks a project, and Done is capped", async () => {
  const setSelect = (el, value) => {
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(el, value);
    el.dispatchEvent(new Event("change", { bubbles: true }));
  };
  named(".footer-links button", "Board").click();
  const rowOf = (name) => document.querySelector(`.board-lane[aria-label="Backlog of ${name}"]`);
  // The project is trusted from its row (it has no hooks to confirm); the row then fills in.
  const untrusted = await until("the untrusted row", () => document.querySelector('.board-lane-untrusted[aria-label="Backlog of pile"]'), 15000);
  withText("button", "Trust project", untrusted).click();
  await until("both rows", () => rowOf("pile") && rowOf("workflow") && !rowOf("pile").classList.contains("board-lane-untrusted"), 20000);
  check("trusting a project from its row fills the row in", all(".board-col", rowOf("pile")).length === 4);
  const doneCards = () => all(".board-col-done .backlog-card", rowOf("pile"));
  check("Done shows five of seven finished issues, with Show all", doneCards().length === 5 && !!withText("button", "Show all 7", rowOf("pile")), `${doneCards().length} cards`);
  withText("button", "Show all 7", rowOf("pile")).click();
  await until("all seven", () => doneCards().length === 7, 4000);
  check("Show all shows them all", doneCards().length === 7 && !!withText("button", "Show fewer", rowOf("pile")));
  withText("button", "Show fewer", rowOf("pile")).click();
  await until("five again", () => doneCards().length === 5, 4000);
  check("and Show fewer goes back to five", doneCards().length === 5);

  // An issue no run touched: its text, no session yet, and markup in it stays text.
  withText(".issue-title", "Finished issue 1", rowOf("pile")).click();
  const bare = await until("the finished issue's panel", () => document.querySelector(".issue-panel-text")?.textContent.includes("Done.") && document.querySelector(".issue-panel"), 8000);
  check("an issue no run touched says no session worked on it", bare.textContent.includes("No session has worked on it yet."), bare.textContent.slice(0, 120));
  await sleep(300);
  check("markup in an issue's text is shown as text and never runs", !window.pwned && !bare.querySelector("img") && bare.querySelector(".issue-panel-text").textContent.includes("<img"), bare.querySelector(".issue-panel-text").innerHTML.slice(0, 160));
  bare.querySelector('button[aria-label="Close"]').click();
  await until("closed", () => !document.querySelector(".issue-panel"), 4000);

  // A row collapses: its columns go, its header and counts stay, and it stays so.
  named("button", "Collapse pile", rowOf("pile")).click();
  await until("the row folded", () => all(".board-col", rowOf("pile")).length === 0, 4000);
  check("a collapsed row keeps its header and counts", !!rowOf("pile").querySelector(".board-lane-title strong") && rowOf("pile").textContent.includes("0 open") && !!named("button", "Expand pile", rowOf("pile")), rowOf("pile").textContent.slice(0, 80));
  check("the other row is not folded", all(".board-col", rowOf("workflow")).length === 4);
  check("the choice is kept in the browser's storage", (() => { try { return JSON.parse(localStorage.getItem("freeloop:board-collapsed") ?? "[]").length === 1; } catch { return false; } })(), localStorage.getItem("freeloop:board-collapsed"));
  named(".footer-links button", "Queue").click();
  await until("the Queue", () => [...document.querySelectorAll(".page h2")].some((h) => h.textContent === "Queue"), 8000);
  named(".footer-links button", "Board").click();
  await until("the Board again", () => rowOf("pile"), 8000);
  check("it is still folded when the Board is opened again", all(".board-col", rowOf("pile")).length === 0 && !!named("button", "Expand pile", rowOf("pile")));

  // The filter lists the projects that have a row, and shows one or all.
  const filter = await until("the filter", () => document.querySelector(".board-filter select"), 4000);
  check("the filter lists all projects and each one with a row", [...filter.options].map((o) => o.textContent).join("|") === "All projects|pile|workflow", [...filter.options].map((o) => o.textContent).join("|"));
  setSelect(filter, [...filter.options].find((o) => o.textContent === "workflow").value);
  await until("one row", () => !rowOf("pile") && rowOf("workflow"), 4000);
  check("the filter shows one project's row", !rowOf("pile") && !!rowOf("workflow"));
  setSelect(filter, "");
  await until("both rows again", () => rowOf("pile") && rowOf("workflow"), 4000);
  check("and all of them again", !!rowOf("pile") && !!rowOf("workflow"));
  named("button", "Expand pile", rowOf("pile")).click();
  await until("the row open", () => all(".board-col", rowOf("pile")).length === 4, 4000);
  check("an expanded row has its columns again", all(".board-col", rowOf("pile")).length === 4);
});

// However short the window, only the parts scroll, never the page: text for screen
// readers below a scrolled list once stretched it.
await step("the page itself never scrolls", async () => {
  const pageFits = (where) => check(`the page fits the window on ${where}`, document.scrollingElement.scrollHeight <= window.innerHeight, `${document.scrollingElement.scrollHeight} > ${window.innerHeight}`);
  openSession("Check long");
  await until("the conversation", () => document.querySelector(".conversation"));
  await sleep(300);
  pageFits("a long session");
  for (const name of ["Usage", "Help"]) {
    named(".footer-links button", name).click();
    await sleep(800);
    pageFits(name);
  }
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

// A pane that fails to draw leaves the rest of the window working (GUI-017): the session
// "Check boom" holds a tool call that throws when drawn (scripts/appcheck.sh).
await step("a pane that fails leaves the window working", async () => {
  openSession("Check alpha");
  await until("the conversation", () => document.querySelector(".conversation"));
  document.querySelector('.layout-switch button[aria-label="2 panes"]').click();
  await until("two panes", () => all(".pane").length === 2);
  // A request waits in the first pane, then the second pane shows the broken session.
  const first = document.querySelector('.pane[data-position="0"]');
  const box = await until("the message box", () => first.querySelector(".composer textarea"));
  type(box, "go");
  await sleep(50);
  (await until("Send", () => { const b = named(".composer-actions button.primary", "Send", first); return b && !b.disabled && b; })).click();
  const endsBefore = all(".turn-stop", first).length;
  await until("a request", () => first.querySelector(".approval-buttons button.primary"), 30000);
  // The session is the oldest, so no pane offers it: a click in the second pane makes it
  // the active one, and the sidebar's row opens the session there.
  document.querySelector('.pane[data-position="1"]').dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
  await sleep(100);
  openSession("Check boom");
  const failed = await until("the failure in its pane", () => document.querySelector('.pane[data-position="1"] .pane-failed')).catch((e) => {
    throw new Error(`${e.message}: ${all(".pane").map((p) => p.innerHTML.length + " tools=" + p.querySelectorAll("[data-tool-call]").length + " " + p.textContent.slice(0, 40)).join(" | ")}`);
  });
  check("the failing pane says so, with a report and a way to close", failed.getAttribute("role") === "alert" && withText("button", "Copy a report", failed) && withText("button", "Close", failed), failed.textContent);
  check("the window's own error screen is not shown", !document.querySelector("#root > .empty"));
  check("the other pane and the waiting approval go on", !!first.querySelector(".approval-buttons button.primary") && !first.querySelector(".pane-failed"));
  check("the sidebar goes on", all(".session-row").length > 3 && !!withText(".session-row", "Check alpha"));
  // Allowing it, and whatever the demo asks next, works from the pane beside the failed one.
  await until("the turn to end", () => {
    first.querySelector(".approval-buttons button.primary")?.click();
    return all(".turn-stop", first).length > endsBefore;
  }, 30000);
  withText("button", "Close", failed).click();
  await until("one pane", () => all(".pane").length === 1);
  check("closing the failed pane leaves the other one", !!document.querySelector(".pane .session-header"));
  document.querySelector('.layout-switch button[aria-label="One pane"]').click();
  await sleep(200);
});

// The built page carries a policy that allows no inline script and no source but the app's
// own (and the preview's localhost frames), and nothing inline has run under it (GUI-017).
await step("the page carries its Content Security Policy", async () => {
  const policy = document.querySelector('meta[http-equiv="Content-Security-Policy"]')?.content ?? "";
  const directive = (name) => policy.split(";").map((d) => d.trim()).find((d) => d.startsWith(`${name} `)) ?? "";
  check("scripts come from the app alone", directive("script-src") === "script-src 'self'", directive("script-src"));
  check("nothing else is the default", directive("default-src") === "default-src 'self'", directive("default-src"));
  check("frames are the preview's localhost and the PDF viewer's blob", /^frame-src blob: (https?:\/\/(localhost|127\.0\.0\.1|\[::1\]):\* ?)+/.test(directive("frame-src")) && !directive("frame-src").includes("*.") && !/\s\*(\s|$)/.test(directive("frame-src")), directive("frame-src"));
  check("no plug-ins, no base, no forms", ["object-src 'none'", "base-uri 'none'", "form-action 'none'"].every((d) => policy.includes(d)));
  check("no inline script is allowed", !policy.includes("'unsafe-inline'") || !directive("script-src").includes("'unsafe-inline'"));
  check("the page has no inline script", all("script").every((s) => s.src));
});

return results;
