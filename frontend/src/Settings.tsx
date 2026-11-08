import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { ask } from "./ask.ts";
import { autoSaved, settingsKey, SETTINGS_GROUPS, type GeneralPage } from "./settings.ts";
import ShortcutsEditor from "./ShortcutsEditor.tsx";
import ColorEditor from "./ColorEditor.tsx";
import SourceMarks from "./SourceMarks.tsx";
import FontEditor from "./FontEditor.tsx";
import type { ColorSet } from "./colors.ts";
import type { FontChoice } from "./fonts.ts";
import { bindingOf, keyLabel } from "./shortcuts.ts";
import { Browser, Clipboard } from "@wailsio/runtime";
import { PriceTable, UsageTable } from "./Costs.tsx";
import BudgetTable from "./BudgetTable.tsx";
import { HookTable, PendingCommands } from "./Hooks.tsx";
import { groupPending, pendingLabel } from "./pending.ts";
import { EngineService, type Blocked, type ConfigPreview, type ConfigRestore, type Config, type UpdateInfo, type EnvironmentInfo, type Profile, type ProjectInfo, type ProviderInfo } from "./api.ts";

interface Props {
  // Which of the app's pages to show; one form holds them all, so moving between
  // them keeps what waits for Save.
  page: GeneralPage;
  config: Config;
  providers: ProviderInfo[];
  harnesses: { id: string; name: string }[];
  environments: EnvironmentInfo[];
  profiles: Profile[];
  projects: ProjectInfo[];
  onProjectsChanged: () => void;
  onImport: () => void;
  onProviders: () => void;
  colors: ColorSet;
  onColors: (c: ColorSet) => void;
  onFonts: (f: FontChoice) => void;
  onSaved: (c: Config) => void;
  onError: (err: unknown) => void;
  // The session a problem report describes: the one last open, if any.
  reportSession?: { id: string; title: string } | null;
}

// What restoring a backup would do, before anything changes (SET-005).
function RestorePreview({ preview, onRestore, onCancel }: { preview: ConfigPreview; onRestore: () => void; onCancel: () => void }) {
  const files = preview.files ?? [];
  const count = (change: string) => files.filter((f) => f.change === change).length;
  const gone = preview.gone ?? [];
  const addresses = preview.addresses ?? [];
  const waiting = preview.needApproval ?? [];
  const keys = preview.needKeys ?? [];
  // It opens under the button, at the bottom of a long page.
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    box.current?.scrollIntoView({ block: "nearest" });
  }, []);
  return (
    <div className="restore-preview" ref={box} role="group" aria-label="What restoring this backup would do">
      <h4>Restore this backup?</h4>
      <p className="muted">Nothing has changed yet. The files it replaces are kept in the data folder's backups folder, and local-only mode stays on if it is on.</p>
      <details>
        <summary>
          {files.length} files: {count("replaced")} replaced, {count("new")} new, {count("same")} unchanged
        </summary>
        <ul>
          {files.map((f) => (
            <li key={f.path}>
              <code>{f.path}</code> <span className="muted">{f.change}</span>
            </li>
          ))}
        </ul>
      </details>
      {gone.length > 0 && (
        <>
          <p className="error-text">These would stop applying, because the backup's files do not have them:</p>
          <ul>
            {gone.map((g) => (
              <li key={g}>{g}</li>
            ))}
          </ul>
        </>
      )}
      {addresses.length > 0 && (
        <>
          <p>Where things would be sent:</p>
          <ul>
            {addresses.map((a) => (
              <li key={a.what}>
                {a.what}: {a.from ? <><code>{a.from}</code> becomes </> : "new, "}
                <code>{a.to}</code>
              </li>
            ))}
          </ul>
        </>
      )}
      {waiting.length > 0 && (
        <>
          <p>These {waiting.length} would wait for your approval, and do nothing until you give it:</p>
          <div className="pending-list" tabIndex={0} role="group" aria-label={`${waiting.length} would wait for approval`}>
            {groupPending(waiting).map((g) => (
              <div key={g.title}>
                <h5>{g.title}</h5>
                <ul>
                  {g.items.map((c, i) => (
                    <li key={i}>
                      {pendingLabel(c)} from {c.source}: <code>{c.runs}</code>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </>
      )}
      {keys.length > 0 && <p>You would enter the API keys for {keys.join(", ")} under Providers.</p>}
      <div className="row">
        <button type="button" className="primary" onClick={onRestore}>
          Restore
        </button>
        <button type="button" onClick={onCancel}>
          Cancel
        </button>
      </div>
    </div>
  );
}

// waitingDraft holds the edits that wait for Save (hooks, prices, budgets, web search)
// while the user is on another page, so leaving the settings does not drop them.
let waitingDraft: Pick<Config, "hooks" | "prices" | "budgets" | "web"> | null = null;

export default function Settings({ page, config, providers, harnesses, environments, profiles, projects, onProjectsChanged, onImport, onProviders, colors, onColors, onFonts, onSaved, onError, reportSession }: Props) {
  // A problem report: copied, and shown when asked, before it is pasted anywhere.
  const [report, setReport] = useState<string | null>(null);
  const [reportCopied, setReportCopied] = useState(false);
  const copyReport = () =>
    EngineService.ProblemReport(reportSession?.id ?? "")
      .then((text) => Clipboard.SetText(text))
      .then(() => setReportCopied(true))
      .catch(onError);
  const toggleReport = () =>
    report !== null
      ? setReport(null)
      : EngineService.ProblemReport(reportSession?.id ?? "")
          .then(setReport)
          .catch(onError);
  // What waited for Save when the settings were last left comes back with them.
  const [form, setForm] = useState<Config>(() => (waitingDraft ? { ...config, ...waitingDraft } : config));
  const [saved, setSaved] = useState(false);
  // Connections local-only mode refused (NFR-OFF-003).
  const [blocked, setBlocked] = useState<Blocked[]>([]);
  useEffect(() => {
    EngineService.BlockedConnections()
      .then((b) => setBlocked(b ?? []))
      .catch(onError);
  }, [config]);
  // Config backup (SET-005).
  const [backup, setBackup] = useState<string | null>(null);
  const [restored, setRestored] = useState<ConfigRestore | null>(null);
  const [preview, setPreview] = useState<ConfigPreview | null>(null);
  const backUp = () =>
    EngineService.ExportConfig()
      .then((path) => path && setBackup(path))
      .catch(onError);
  const chooseBackup = () => {
    setRestored(null);
    EngineService.PreviewConfigRestore()
      .then((p) => setPreview(p))
      .catch(onError);
  };
  const restore = (p: ConfigPreview) =>
    EngineService.RestoreConfig(p.fingerprint)
      .then((r) => {
        setPreview(null);
        if (r) setRestored(r);
      })
      .catch(onError);
  // ⌘+ and ⌘- change one field, and the fonts save themselves; the rest of the
  // form may hold unsaved edits. Approving a command is not an edit either.
  const { ui: { fontSize: savedFontSize, textFont: savedTextFont, codeFont: savedCodeFont, ...savedUi }, commandApprovals: _approvals, ...savedRest } = config;
  // What the page saved itself comes back as a new config, and must not undo what is
  // being typed elsewhere on the page.
  const ownSave = useRef("");
  // Not on opening: the form starts from the config already, with any draft kept.
  const opened = useRef(true);
  useEffect(() => {
    if (opened.current) {
      opened.current = false;
      return;
    }
    if (ownSave.current !== settingsKey(config)) setForm(config);
  }, [JSON.stringify([savedRest, savedUi])]);
  useEffect(
    () => setForm((f) => ({ ...f, ui: { ...f.ui, fontSize: savedFontSize, textFont: savedTextFont, codeFont: savedCodeFont } })),
    [savedFontSize, savedTextFont, savedCodeFont],
  );
  // The opt-in update check (OQ-015).
  const [update, setUpdate] = useState<UpdateInfo | null>(null);
  const [checking, setChecking] = useState(false);
  useEffect(() => {
    EngineService.UpdateStatus().then(setUpdate).catch(onError);
  }, [config]);
  const checkNow = () => {
    setChecking(true);
    EngineService.CheckForUpdate()
      .then(setUpdate)
      .catch(() => EngineService.UpdateStatus().then(setUpdate))
      .finally(() => setChecking(false));
  };

  const save = (e: FormEvent) => {
    e.preventDefault();
    EngineService.SaveSettings(form)
      .then((c) => {
        onSaved(c);
        setSaved(true);
        setTimeout(() => setSaved(false), 1500);
      })
      .catch(onError);
  };

  // Changes save themselves, as in a Mac settings window, once the field being typed
  // in is left. Leaving the page saves too: it used to drop them without a word.
  const formBox = useRef<HTMLFormElement>(null);
  const [left, setLeft] = useState(0);
  const saveByItself = (f: Config, c: Config) => {
    const next = autoSaved(f, c);
    if (settingsKey(next) === settingsKey(c)) return null;
    return EngineService.SaveSettings(next);
  };
  const current = useRef({ form, config });
  current.current = { form, config };
  useEffect(() => {
    const timer = setTimeout(() => {
      const typing = document.activeElement;
      if (typing && formBox.current?.contains(typing) && typing.matches('input:not([type="checkbox"]), textarea')) return;
      saveByItself(form, config)
        ?.then((c) => {
          if (!c) return;
          ownSave.current = settingsKey(c);
          onSaved(c);
          // What waits for the Save button stays as typed.
          setForm((f) => ({ ...c, hooks: f.hooks, prices: f.prices, budgets: f.budgets, web: f.web }));
          setSaved(true);
          setTimeout(() => setSaved(false), 1500);
        })
        .catch(onError);
    }, 250);
    return () => clearTimeout(timer);
  }, [form, left]);
  useEffect(
    () => () => {
      const { form: f, config: c } = current.current;
      saveByItself(f, c)
        ?.then((saved) => saved && onSaved(saved))
        .catch(onError);
      // Leaving for another page keeps what waits for Save, for the app's run.
      const waiting = { hooks: f.hooks, prices: f.prices, budgets: f.budgets, web: f.web };
      waitingDraft = JSON.stringify(waiting) === JSON.stringify({ hooks: c.hooks, prices: c.prices, budgets: c.budgets, web: c.web }) ? null : waiting;
    },
    [],
  );
  const unsaved = settingsKey(form) !== settingsKey(autoSaved(form, config));
  // What waits for Save, by name, wherever it is.
  const same = (a: unknown, b: unknown) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null);
  const waitingNames = [
    !same(form.hooks, config.hooks) && "Hooks",
    !same(form.prices, config.prices) && "Prices",
    !same(form.budgets, config.budgets) && "Budgets",
    !same(form.web, config.web) && "Web search",
  ].filter((x): x is string => !!x);
  const waitingCount = waitingNames.length;
  const waiting = waitingNames.join(", ").replace(/, ([^,]*)$/, " and $1") || "Nothing";
  const allowBypass = async (on: boolean) => {
    if (on) {
      const ok = await ask(
        "Allow bypass mode? In bypass mode tool calls run without asking. Protected paths and deny rules still hold, but shell commands can do anything you can. New sessions never start in bypass.",
      );
      if (!ok) return;
    }
    setForm((f) => ({ ...f, permissions: { ...f.permissions, allowBypass: on } }));
  };

  const provider = providers.find((p) => p.id === form.defaults.providerId);

  const title = SETTINGS_GROUPS.flatMap((g) => g.pages).find((p) => p.id === page)?.label ?? "Settings";
  const pageForm = (inner: ReactNode) => (
    <form ref={formBox} className="card form" onSubmit={save} onBlur={() => setLeft((n) => n + 1)}>
      {inner}
      <div className={unsaved ? "row settings-save" : "row"} role="status">
        {unsaved ? (
          <>
            <span className="muted">{waiting} {waitingCount === 1 ? "waits" : "wait"} for Save.</span>
            <button type="submit" className="primary">
              Save
            </button>
            <button type="button" onClick={() => setForm(config)}>
              Revert
            </button>
          </>
        ) : (
          <span className="muted">Changes save themselves.</span>
        )}
        {saved && <span className="ok">Saved</span>}
      </div>
    </form>
  );

  return (
    <section className="page settings-page">
      <h2>{title}</h2>
      {page === "appearance" &&
        pageForm(
          <>
        <h3>Theme and text size</h3>
        <label>
          Theme
          <select value={form.ui.theme} onChange={(e) => setForm({ ...form, ui: { ...form.ui, theme: e.target.value } })}>
            <option value="system">Follow the system</option>
            <option value="light">Light</option>
            <option value="dark">Dark</option>
          </select>
        </label>
        <label>
          Font size: {form.ui.fontSize} pt
          <input
            type="range"
            min={11}
            max={20}
            value={form.ui.fontSize}
            onChange={(e) => setForm({ ...form, ui: { ...form.ui, fontSize: Number(e.target.value) } })}
          />
          <span className="muted">
            {keyLabel(bindingOf("textLarger"))} and {keyLabel(bindingOf("textSmaller"))} change this from anywhere in the app.
          </span>
        </label>

        <h3>Fonts</h3>
        <FontEditor textFont={config.ui.textFont} codeFont={config.ui.codeFont} onChange={onFonts} />

        <h3>Colors</h3>
        <ColorEditor colors={colors} onChange={onColors} onError={onError} />

        <h3>Provider marks</h3>
        <p className="muted">What each session runs on shows as a colour and two letters, in the sidebar and on its pane.</p>
        <SourceMarks
          value={form.ui.sourceMarks as Record<string, { slot: number; letters: string }> | null | undefined}
          providers={providers}
          harnesses={harnesses}
          onChange={(sourceMarks) => setForm({ ...form, ui: { ...form.ui, sourceMarks } })}
        />

          </>,
        )}
      {page === "keyboard" &&
        pageForm(
          <>
        <ShortcutsEditor value={form.ui.shortcuts ?? {}} onChange={(shortcuts) => setForm({ ...form, ui: { ...form.ui, shortcuts } })} />
        <p className="muted">⌘1 to ⌘9 open the sessions listed first, and ⌘↩, ⌘⇧↩ and ⌘⇧⌫ answer an approval from anywhere; those stay as they are.</p>

          </>,
        )}
      {page === "sessions" &&
        pageForm(
          <>
        <h3>Defaults for new sessions</h3>
        <label>
          Provider
          <select
            value={form.defaults.providerId}
            onChange={(e) => {
              const p = providers.find((x) => x.id === e.target.value);
              setForm({ ...form, defaults: { ...form.defaults, providerId: e.target.value, model: p?.defaultModel ?? "" } });
            }}
          >
            <option value="">First in the list</option>
            {providers.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Model
          <input
            value={form.defaults.model}
            placeholder={provider?.defaultModel || "The provider's default"}
            onChange={(e) => setForm({ ...form, defaults: { ...form.defaults, model: e.target.value } })}
          />
        </label>
        <label>
          Environment <span className="muted">(for projects without their own)</span>
          <select
            value={form.defaults.environmentId}
            onChange={(e) => setForm({ ...form, defaults: { ...form.defaults, environmentId: e.target.value } })}
          >
            <option value="">Local</option>
            {environments
              .filter((x) => x.id !== "local")
              .map((x) => (
                <option key={x.id} value={x.id}>
                  {x.name}
                </option>
              ))}
          </select>
        </label>
        <label>
          Profile <span className="muted">(for projects without their own)</span>
          <select value={form.defaults.profileId} onChange={(e) => setForm({ ...form, defaults: { ...form.defaults, profileId: e.target.value } })}>
            <option value="">Default</option>
            {profiles
              .filter((x) => x.id !== "default")
              .map((x) => (
                <option key={x.id} value={x.id}>
                  {x.name}
                </option>
              ))}
          </select>
        </label>

        <h3>Notifications</h3>
        <p className="muted">Shown while Freeloop is in the background; clicking one opens its session.</p>
        <label className="row">
          <input
            type="checkbox"
            checked={form.notifications.enabled}
            onChange={(e) => setForm({ ...form, notifications: { ...form.notifications, enabled: e.target.checked } })}
          />
          When a session needs an approval or fails
        </label>
        <label className="row">
          <input
            type="checkbox"
            disabled={!form.notifications.enabled}
            checked={form.notifications.finished}
            onChange={(e) => setForm({ ...form, notifications: { ...form.notifications, finished: e.target.checked } })}
          />
          Also when a session finishes a turn
        </label>
        {projects.length > 0 && (
          <fieldset>
            <legend>Projects <span className="muted">(saved at once)</span></legend>
            {projects.map((p) => (
              <label key={p.id} className="row">
                <input
                  type="checkbox"
                  checked={!p.muted}
                  onChange={(e) =>
                    EngineService.SetProjectMuted(p.id, !e.target.checked)
                      .then(onProjectsChanged)
                      .catch(onError)
                  }
                />
                {p.name} <span className="muted">{p.muted ? "muted" : ""}</span>
              </label>
            ))}
          </fieldset>
        )}

          </>,
        )}
      {page === "costs" && (
        <>
          {pageForm(
            <>
        <h3 id="prices">Prices</h3>
        <PriceTable prices={form.prices ?? []} providers={providers} onChange={(prices) => setForm({ ...form, prices })} />

        <h3 id="budgets">Budgets</h3>
        <BudgetTable budgets={form.budgets ?? []} profiles={profiles} onChange={(budgets) => setForm({ ...form, budgets })} />

            </>,
          )}
      <div className="card">
        <h3>Usage and estimated cost</h3>
        <UsageTable
          version={config}
          onError={onError}
          onPrice={(providerId, model) => {
            setForm({ ...form, prices: [...(form.prices ?? []), { providerId, model, input: 0, output: 0, cacheRead: 0, cacheWrite: 0, cacheWrite1h: 0, longPromptOver: 0 }] });
            requestAnimationFrame(() => document.getElementById("prices")?.scrollIntoView({ block: "start" }));
          }}
        />
      </div>
        </>
      )}
      {page === "permissions" && (
        <>
          <PendingCommands version={[config, restored]} onSaved={onSaved} onError={onError} />
          {pageForm(
            <>
        <h3>Bypass mode</h3>
        <label className="row">
          <input
            type="checkbox"
            checked={form.permissions.allowBypass}
            onChange={(e) => allowBypass(e.target.checked)}
          />
          Allow bypass mode to be chosen for a session
        </label>

        <h3>Hooks</h3>
        <HookTable hooks={form.hooks ?? []} onChange={(hooks) => setForm({ ...form, hooks })} />

            </>,
          )}
        </>
      )}
      {page === "network" &&
        pageForm(
          <>
        <h3>Local only</h3>
        <label className="row">
          <input
            type="checkbox"
            checked={!!form.network?.localOnly}
            onChange={(e) => setForm({ ...form, network: { ...form.network, localOnly: e.target.checked } })}
          />
          Local only: the app connects to nothing but this Mac
        </label>
        <p className="muted">
          Providers and MCP servers elsewhere become unreachable; local ones (Ollama, LM Studio, servers on localhost) keep working. Docker
          containers created while it is on have no network. Commands, external agents and MCP servers the app starts run as you, outside this
          switch.
        </p>
        {blocked.length > 0 && (
          <details>
            <summary>{blocked.length} blocked connection{blocked.length > 1 ? "s" : ""}</summary>
            <ul>
              {[...blocked].reverse().map((b, i) => (
                <li key={i}>
                  {b.host} <span className="muted">{new Date(b.at).toLocaleString()}</span>
                </li>
              ))}
            </ul>
          </details>
        )}

        <h3>Web search</h3>
        <p className="muted">
          The built-in agent's <code>web_search</code> tool queries this endpoint, and the tool is offered only when one is set: Freeloop has no search
          engine of its own to send your queries to. A self-hosted SearXNG and the Brave Search API both work. Each search asks you first.
        </p>
        <label>
          Search address <span className="muted">(with {"{query}"} where the query goes, and {"{key}"} if the key goes in the address)</span>
          <input
            value={form.web?.searchURL ?? ""}
            placeholder="http://localhost:8888/search?format=json&q={query}"
            onChange={(e) => setForm({ ...form, web: { ...form.web, searchURL: e.target.value } })}
          />
        </label>
        <label>
          Key header <span className="muted">(optional, such as X-Subscription-Token)</span>
          <input value={form.web?.searchHeader ?? ""} onChange={(e) => setForm({ ...form, web: { ...form.web, searchHeader: e.target.value } })} />
        </label>
        {(form.web?.searchHeader || form.web?.searchURL?.includes("{key}")) && (
          <label>
            Key <span className="muted">(kept in the Keychain; saved at once)</span>
            <input
              type="password"
              autoComplete="off"
              placeholder="Leave empty to keep the current key"
              onBlur={(e) => {
                const key = e.target.value.trim();
                if (!key) return;
                e.target.value = "";
                EngineService.SetWebSearchKey(key).catch(onError);
              }}
            />
          </label>
        )}

        <h3>Updates</h3>
        <label className="row">
          <input
            type="checkbox"
            checked={!!form.updates?.check}
            onChange={(e) => setForm({ ...form, updates: { ...form.updates, check: e.target.checked } })}
          />
          Check for a newer version once a day
        </label>
        <p className="muted">
          Asks GitHub for the project's latest release and says when it is newer than {update?.current ?? "this version"}. The request carries
          nothing about this copy of the app, and nothing is downloaded or installed. Off unless you turn it on.
        </p>
        <div className="row">
          <button type="button" disabled={checking || !!form.network?.localOnly} onClick={checkNow}>
            {checking ? "Checking…" : "Check now"}
          </button>
          {update?.newer && update.url && (
            <span>
              Version {update.latest} is available.{" "}
              <a
                href={update.url}
                onClick={(e) => {
                  e.preventDefault();
                  Browser.OpenURL(update.url!).catch(onError);
                }}
              >
                See the release
              </a>
            </span>
          )}
          {update && !update.newer && update.latest && <span className="muted">Up to date ({update.current}).</span>}
          {update?.error && <span className="muted">{update.error}</span>}
        </div>

          </>,
        )}
      {page === "data" && (
        <>
      <div className="card">
        <h3>Import from Claude Code</h3>
        <p className="muted">Copy sessions, MCP servers, permission rules and skills from Claude Code. Nothing is written back to it.</p>
        <div className="row">
          <button type="button" onClick={onImport}>
            Import from Claude Code…
          </button>
        </div>

      </div>
      <div className="card">
        <h3>Backup</h3>
        <p className="muted">
          Settings, providers, profiles, environments, rules, MCP servers and harness presets in one file. API keys and other secrets stay in the
          Keychain and are not included; values you typed into config files as plain text are.
        </p>
        <div className="row">
          <button type="button" onClick={backUp}>
            Back up configuration…
          </button>
          <button type="button" onClick={chooseBackup}>
            Restore from a backup…
          </button>
          {backup && <span className="ok">Saved to {backup}</span>}
        </div>
        {preview && <RestorePreview preview={preview} onRestore={() => restore(preview)} onCancel={() => setPreview(null)} />}
        {restored && (
          <div className="banner" role="status">
            Restored {restored.files?.length ?? 0} files. {restored.backupNote}
            {restored.needApproval > 0 && <p>What waits for your approval, listed at the top of this page, does nothing until you approve it.</p>}
            {(restored.gone ?? []).length > 0 && (
              <p>
                These no longer apply, because the backup's files do not have them: {(restored.gone ?? []).join("; ")}.
              </p>
            )}
            {(restored.needKeys ?? []).length > 0 && (
              <p>
                Enter the API keys for {(restored.needKeys ?? []).join(", ")} under{" "}
                <button type="button" className="link" onClick={onProviders}>
                  Providers
                </button>
                .
              </p>
            )}
          </div>
        )}
      </div>

      <div className="card">
        <h3>Report a problem</h3>
        <p className="muted">
          Copies Freeloop's version, this Mac's model and macOS version,{" "}
          {reportSession ? <>the agent and provider of “{reportSession.title}”,</> : "the agent and provider of the session last open,"} and the log's last 200
          lines, for an issue. Secrets the app knows and your home folder's path are taken out, and nothing is sent anywhere. Read it before you share it.
        </p>
        <div className="row">
          <button type="button" onClick={copyReport}>
            Copy a report
          </button>
          <button type="button" onClick={toggleReport} aria-expanded={report !== null}>
            {report !== null ? "Hide what is copied" : "Show what is copied"}
          </button>
          <button type="button" onClick={() => EngineService.ShowLogs().catch(onError)}>
            Show logs in Finder
          </button>
          {reportCopied && <span className="ok" role="status">Copied to the clipboard.</span>}
        </div>
        {report !== null && (
          <pre className="report-preview" tabIndex={0} aria-label="The report">
            {report}
          </pre>
        )}
      </div>

        </>
      )}
    </section>
  );
}
