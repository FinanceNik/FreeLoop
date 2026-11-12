// The second pass of the app check: the same app on a data folder set up the way a
// user's looks after weeks of use, not a fresh one (scripts/appcheck.sh, "lived-in").
// A default profile on an external agent, a global rule allowing shell, dozens of
// skills with long descriptions, a default token optimizer, a hook written by hand, an
// MCP server that cannot start, many sessions. Bugs it was written for passed the fresh
// pass and broke here: the demo ran on the default profile's agent, and the palette
// filled with the agent's skills. Helpers are in lib.js.

await until("the session list", () => all(".session-row").length >= 30, 20000);

await step("a lived-in app opens without errors", async () => {
  await sleep(1500);
  const errors = all(".banner.error").map((b) => b.textContent.trim());
  check("no error banner", errors.length === 0, errors.join(" | "));
  const problems = withText(".banner.warn", "Configuration problems");
  check("the configuration reads without problems", !problems, problems?.textContent);
  check("every session is listed", all(".session-row").length >= 30, `${all(".session-row").length} rows`);
});

await step("a global rule allows any shell command", async () => {
  // Added in the app, as a user does: one written into rules.yaml by hand waits for approval.
  await openPalette("Policy");
  (await until("the entry", () => all(".palette-item").find((i) => i.querySelector(".palette-title")?.textContent === "Policy"), 4000)).click();
  const effect = await until("the rule form", () => named("select", "Effect"), 8000);
  const choose = (el, value) => {
    Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(el, value);
    el.dispatchEvent(new Event("change", { bubbles: true }));
  };
  choose(effect, "allow");
  choose(named("select", "Tool"), "shell");
  await sleep(100);
  withText("button", "Add rule").click();
  await until("the rule", () => all(".rule-list li, ul li").some((li) => /allow\s+shell/i.test(li.textContent)), 5000);
  check("the global rule is in place", true);
});

// The made-up long session (80,000 events, issue 040): the time from a click on it to
// its end being drawn is printed, so the window's cost of a long session is seen. It
// is a number to record, not yet a limit (backlog/prds/before-going-public.md).
await step("the long session opens, and the time to its end is printed", async () => {
  const t0 = performance.now();
  openSession("Long session");
  const pane = await until("the long session", () => activePane().querySelector(".session-titleline h2")?.textContent === "Long session" && activePane(), 8000);
  // The end is drawn when the last turn's stop line is there and the view sits at the bottom.
  const scroller = () => pane.querySelector(".conversation");
  await until("the end of the long session", () => {
    const c = scroller();
    return c && all(".turn-stop", pane).length > 100 && toTheEnd(c) < 40;
  }, 120000);
  const ms = Math.round(performance.now() - t0);
  check(`the long session's end is drawn after ${ms} ms, ${all(".turn-stop", pane).length} turns`, true);
});

await step("the palette stays short with many skills and sessions", async () => {
  openSession("Work 30");
  await until("a session", () => activePane().querySelector(".session-titleline h2")?.textContent === "Work 30", 8000);
  const box = await openPalette("");
  const rest = all(".palette-heading").map((h) => h.textContent);
  check("before typing, only the short sections", rest.every((h) => ["Waiting for you", "Actions", "Recent sessions", "Go to"].includes(h)), rest.join(", "));
  check("at most five recent sessions", all('.palette-section[aria-label="Recent sessions"] .palette-item').length <= 5);
  for (const q of ["se", "data", "review", "a"]) {
    await openPalette(q);
    const big = all(".palette-section").filter((s) => s.querySelectorAll(".palette-item").length > 6);
    check(`a search for "${q}" shows a few of each group`, big.length === 0, big.map((s) => s.getAttribute("aria-label")).join(", "));
  }
  await openPalette("/");
  const slash = all(".palette-heading").map((h) => h.textContent);
  check("a slash lists only the agent's commands", slash.every((h) => h === "Agent commands"), slash.join(", "));
  box.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));
  await until("the palette to close", () => !document.querySelector(".palette"), 4000);
});

await step("the demo runs the tour, not the default profile's agent", async () => {
  await openPalette("Try the demo");
  (await until("the palette entry", () => withText(".palette-item", "Try the demo"), 4000)).click();
  const banner = await until("the demo banner", () => withText(".demo-banner", "This is the demo"), 20000);
  check("the demo has its banner", !!banner);
  const pane = activePane();
  const header = pane.querySelector(".session-header")?.textContent ?? "";
  check("the demo runs on the tour", header.includes("freeloop-tour") && !/OpenCode\s·/.test(header), header.slice(0, 120));
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
  check("the tour asks about notes.md and .env", asked.length === 2 && asked.some((t) => t.includes("notes.md")) && asked.some((t) => t.includes(".env")), asked.map((t) => t.slice(0, 60)).join(" | "));
  const refused = all(".conversation .decision-denied", pane).map((d) => d.textContent.trim());
  check("git push is refused by the rule, over the global allow", refused.includes("denied by rule"), refused.join(", "));
  check("the hook is refused as a protected path", refused.includes("denied by protected_path"), refused.join(", "));

  const real = window.fetch;
  window.fetch = (u, o) =>
    decodeURIComponent(`${u}${o?.body ?? ""}`).includes("Remove the demo?")
      ? Promise.resolve(new Response(JSON.stringify("OK"), { status: 200, headers: { "Content-Type": "application/json" } }))
      : real(u, o);
  try {
    withText(".demo-banner button", "Remove demo").click();
    await until("the demo to go", () => !all(".session-row").some((r) => r.textContent.includes("Demo: your policy at work")), 8000);
    check("the demo removes itself", true);
  } finally {
    window.fetch = real;
  }
});

await step("a new session starts from an external default profile", async () => {
  await openPalette("New session");
  (await until("the entry", () => all(".palette-item").find((i) => i.querySelector(".palette-title")?.textContent === "New session"), 4000)).click();
  const start = await until("the new-session form", () => withText("button", "Start session"), 8000);
  const form = start.closest("form");
  await sleep(500);
  const errors = all(".banner.error").map((b) => b.textContent.trim());
  check("the form opens without an error", errors.length === 0, errors.join(" | "));
  const profile = all("label", form).find((l) => l.firstChild?.textContent?.trim() === "Profile")?.querySelector("select")?.selectedOptions?.[0]?.textContent ?? "";
  check("the form opens on the default profile", /OpenCode/i.test(profile), profile);
  // With a project chosen, the default agent can start, or the form says why not.
  const project = all("label", form).find((l) => l.firstChild?.textContent?.trim() === "Project")?.querySelector("select");
  Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value").set.call(project, project.options[1].value);
  project.dispatchEvent(new Event("change", { bubbles: true }));
  await sleep(500);
  const cannot = form.querySelector(".error-text")?.textContent ?? "";
  check("the default agent is ready, or the form says why not", !start.disabled || /cannot start/.test(cannot), cannot || "disabled without a reason");
  openSession("Work 30");
  await until("back in a session", () => activePane().querySelector(".session-titleline h2")?.textContent === "Work 30", 8000);
});

// With no project holding a backlog, the Board says so in two lines and offers to start a
// workflow, or to read how they work (GUI-016).
await step("an empty Board offers to start a workflow", async () => {
  named(".footer-links button", "Board").click();
  const page = await until("the Board", () => document.querySelector(".board-page"), 8000);
  check("an empty Board says there is no backlog and how one comes", page.textContent.includes("No project has a backlog yet.") && page.textContent.includes("A workflow turns a brief into those issues"), page.textContent.slice(0, 120));
  check("and offers Start a workflow and the help", !!withText("button", "Start a workflow…", page) && !!withText("button", "How workflows work", page));
  withText("button", "How workflows work", page).click();
  await until("the workflows help", () => document.querySelector(".help-page h1")?.textContent === "Workflows", 8000);
  check("the link opens the Workflows help", document.querySelector(".help-page h1")?.textContent === "Workflows");
  named(".footer-links button", "Board").click();
  withText("button", "Start a workflow…", await until("the Board again", () => document.querySelector(".board-page"), 8000)).click();
  const form = await until("the new-session form", () => withText("label", "Workflow")?.querySelector("select"), 8000);
  check("Start a workflow opens a new session with the workflow chosen", form.value === "plan-slice-afk", form.value);
  openSession("Work 30");
  await until("back in a session", () => activePane().querySelector(".session-titleline h2")?.textContent === "Work 30", 8000);
});

await step("every control has a name", async () => {
  const unnamed = [];
  for (const e of all("button, a[href], summary, [role=tab], [role=menuitem]")) {
    if (!e.getClientRects().length || e.closest("[aria-hidden=true]")) continue;
    if (!(e.getAttribute("aria-label") || e.getAttribute("aria-labelledby") || e.textContent.trim())) unnamed.push(e.outerHTML.slice(0, 120));
  }
  check("every control has an accessible name", unnamed.length === 0, unnamed.join(" | "));
});

return results;
