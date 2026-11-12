import { useEffect, useRef, useState, type FormEvent, type ReactNode } from "react";
import { ask } from "./ask.ts";
import { autoSaved, pageTitle, settingsKey, type GeneralPage } from "./settings.ts";
import { LANGUAGES, list, locale, t, type PlainKey } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";
import ShortcutsEditor from "./ShortcutsEditor.tsx";
import ColorEditor from "./ColorEditor.tsx";
import SourceMarks from "./SourceMarks.tsx";
import FontEditor from "./FontEditor.tsx";
import StylePicker from "./StylePicker.tsx";
import { lookOf } from "./styles.ts";
import type { ColorSet } from "./colors.ts";
import type { FontChoice } from "./fonts.ts";
import { approvalChord, bindingOf, keyLabel } from "./shortcuts.ts";
import { inFileManager, secretStore } from "./platform.ts";
import { Browser, Clipboard } from "@wailsio/runtime";
import { PriceTable, UsageTable } from "./Costs.tsx";
import BudgetTable from "./BudgetTable.tsx";
import { HookTable, PendingCommands } from "./Hooks.tsx";
import { groupPending, pendingLabel } from "./pending.ts";
import { EngineService, type Blocked, type ConfigPreview, type ConfigRestore, type Config, type UpdateInfo, type BuildInfo, type EnvironmentInfo, type Profile, type ProjectInfo, type ProviderInfo, type Style } from "./api.ts";

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
  // Saved styles (GUI-053), and what StylePicker needs to switch between them.
  styles: Style[];
  onApplyStyle: (style: Style | null, confirm?: boolean) => void;
  onChooseStyle: (id: string) => void;
  onStylesChanged: () => void;
  onSaved: (c: Config) => void;
  onError: (err: unknown) => void;
  // The session a problem report describes: the one last open, if any.
  reportSession?: { id: string; title: string } | null;
}

// How a backup's file compares with the one in force, as the engine says it.
const CHANGES: Record<string, PlainKey> = {
  replaced: "settings.restore.change.replaced",
  new: "settings.restore.change.new",
  same: "settings.restore.change.same",
};

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
    <div className="restore-preview" ref={box} role="group" aria-label={t("settings.restore.label")}>
      <h4>{t("settings.restore.title")}</h4>
      <p className="muted">{t("settings.restore.nothingYet")}</p>
      <details>
        <summary>{t("settings.restore.files", { count: files.length, replaced: count("replaced"), added: count("new"), same: count("same") })}</summary>
        <ul>
          {files.map((f) => (
            <li key={f.path}>
              <code>{f.path}</code> <span className="muted">{CHANGES[f.change] ? t(CHANGES[f.change]) : f.change}</span>
            </li>
          ))}
        </ul>
      </details>
      {gone.length > 0 && (
        <>
          <p className="error-text">{t("settings.restore.gone")}</p>
          <ul>
            {gone.map((g) => (
              <li key={g}>{g}</li>
            ))}
          </ul>
        </>
      )}
      {addresses.length > 0 && (
        <>
          <p>{t("settings.restore.addresses")}</p>
          <ul>
            {addresses.map((a) => (
              <li key={a.what}>
                {a.from
                  ? tNodes("settings.restore.addressChanged", { what: a.what, from: <code>{a.from}</code>, to: <code>{a.to}</code> })
                  : tNodes("settings.restore.addressNew", { what: a.what, to: <code>{a.to}</code> })}
              </li>
            ))}
          </ul>
        </>
      )}
      {waiting.length > 0 && (
        <>
          <p>{t("settings.restore.waiting", { count: waiting.length })}</p>
          <div className="pending-list" tabIndex={0} role="group" aria-label={t("settings.restore.waitingLabel", { count: waiting.length })}>
            {groupPending(waiting).map((g) => (
              <div key={g.title}>
                <h5>{g.title}</h5>
                <ul>
                  {g.items.map((c, i) => (
                    <li key={i}>{tNodes("pending.item", { what: pendingLabel(c), source: c.source, runs: <code>{c.runs}</code> })}</li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        </>
      )}
      {keys.length > 0 && <p>{t("settings.restore.needKeys", { providers: list(keys) })}</p>}
      <div className="row">
        <button type="button" className="primary" onClick={onRestore}>
          {t("common.restore")}
        </button>
        <button type="button" onClick={onCancel}>
          {t("common.cancel")}
        </button>
      </div>
    </div>
  );
}

// The built-in agent's search tool, and an address it can query: names, the same in
// every language.
const WEB_SEARCH_TOOL = "web_search";
// The times offered for stopping an idle agent (OQ-053); a file may hold others.
const IDLE_STOPS = ["15m", "30m", "1h", "2h", "4h", "never"];

function idleStopLabel(v: string): string {
  if (v === "never") return t("settings.idle.never");
  const m = /^(\d+)(m|h)$/.exec(v);
  if (!m) return v;
  const n = Number(m[1]);
  if (m[2] === "m") return t("settings.idle.minutes", { n: String(n) });
  return n === 1 ? t("settings.idle.hour") : t("settings.idle.hours", { n: String(n) });
}

const SEARCH_EXAMPLE ="http://localhost:8888/search?format=json&q={query}";

// waitingDraft holds the edits that wait for Save (hooks, prices, budgets, web search)
// while the user is on another page, so leaving the settings does not drop them.
let waitingDraft: Pick<Config, "hooks" | "prices" | "budgets" | "web"> | null = null;

export default function Settings({ page, config, providers, harnesses, environments, profiles, projects, onProjectsChanged, onImport, onProviders, colors, onColors, onFonts, styles, onApplyStyle, onChooseStyle, onStylesChanged, onSaved, onError, reportSession }: Props) {
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
  // ⌘+ and ⌘- change one field, and the fonts and the style chosen save
  // themselves; the rest of the form may hold unsaved edits. Approving a command
  // is not an edit either.
  const { ui: { fontSize: savedFontSize, textFont: savedTextFont, codeFont: savedCodeFont, style: savedStyle, ...savedUi }, commandApprovals: _approvals, ...savedRest } = config;
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
    () => setForm((f) => ({ ...f, ui: { ...f.ui, fontSize: savedFontSize, textFont: savedTextFont, codeFont: savedCodeFont, style: savedStyle } })),
    [savedFontSize, savedTextFont, savedCodeFont, savedStyle],
  );
  // The opt-in update check (OQ-015).
  const [update, setUpdate] = useState<UpdateInfo | null>(null);
  const [checking, setChecking] = useState(false);
  // Which build is running: its version and the commit it was made from.
  const [build, setBuild] = useState<BuildInfo | null>(null);
  useEffect(() => {
    EngineService.AppBuild().then(setBuild).catch(() => setBuild(null));
  }, []);
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

  // Changes save themselves, as in a system settings window, once the field being typed
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
    !same(form.hooks, config.hooks) && t("settings.hooks.title"),
    !same(form.prices, config.prices) && t("settings.prices.title"),
    !same(form.budgets, config.budgets) && t("settings.budgets.title"),
    !same(form.web, config.web) && t("settings.web.title"),
  ].filter((x): x is string => !!x);
  const waiting = t("settings.save.waiting", { count: waitingNames.length || 1, names: list(waitingNames) || t("settings.save.nothing") });
  const allowBypass = async (on: boolean) => {
    if (on) {
      const ok = await ask(t("settings.bypass.ask"));
      if (!ok) return;
    }
    setForm((f) => ({ ...f, permissions: { ...f.permissions, allowBypass: on } }));
  };

  const provider = providers.find((p) => p.id === form.defaults.providerId);

  const title = pageTitle(page);
  const pageForm = (inner: ReactNode) => (
    <form ref={formBox} className="card form" onSubmit={save} onBlur={() => setLeft((n) => n + 1)}>
      {inner}
      <div className={unsaved ? "row settings-save" : "row"} role="status">
        {unsaved ? (
          <>
            <span className="muted">{waiting}</span>
            <button type="submit" className="primary">
              {t("common.save")}
            </button>
            <button type="button" onClick={() => setForm(config)}>
              {t("common.revert")}
            </button>
          </>
        ) : (
          <span className="muted">{t("settings.save.auto")}</span>
        )}
        {saved && <span className="ok">{t("common.saved")}</span>}
      </div>
    </form>
  );

  return (
    <section className="page settings-page">
      <h2>{title}</h2>
      {page === "appearance" &&
        pageForm(
          <>
        <h3>{t("settings.appearance.title")}</h3>
        <label>
          {t("settings.theme")}
          <select value={form.ui.theme} onChange={(e) => setForm({ ...form, ui: { ...form.ui, theme: e.target.value } })}>
            <option value="system">{t("common.followSystem")}</option>
            <option value="light">{t("settings.theme.light")}</option>
            <option value="dark">{t("settings.theme.dark")}</option>
          </select>
        </label>
        {/* GUI-048: applies once saved, a moment after the choice, like Theme. */}
        <label>
          {t("settings.language")}
          <select className="language-select" value={form.ui.language || "system"} onChange={(e) => setForm({ ...form, ui: { ...form.ui, language: e.target.value } })}>
            <option value="system">{t("common.followSystem")}</option>
            {LANGUAGES.map((l) => (
              <option key={l.id} value={l.id} lang={l.id}>
                {l.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          {t("settings.fontSize", { size: form.ui.fontSize })}
          <input
            type="range"
            min={11}
            max={20}
            value={form.ui.fontSize}
            onChange={(e) => setForm({ ...form, ui: { ...form.ui, fontSize: Number(e.target.value) } })}
          />
          <span className="muted">{t("settings.fontSize.keys", { larger: keyLabel(bindingOf("textLarger")), smaller: keyLabel(bindingOf("textSmaller")) })}</span>
        </label>

        <h3>{t("settings.styles.title")}</h3>
        <StylePicker
          styles={styles}
          look={lookOf(colors, config.ui)}
          chosen={config.ui.style}
          onApply={onApplyStyle}
          onChoose={onChooseStyle}
          onChanged={onStylesChanged}
          onError={onError}
        />

        <h3>{t("settings.fonts.title")}</h3>
        <FontEditor textFont={config.ui.textFont} codeFont={config.ui.codeFont} onChange={onFonts} />

        <h3>{t("settings.colors.title")}</h3>
        <ColorEditor colors={colors} onChange={onColors} onError={onError} />

        <h3>{t("settings.marks.title")}</h3>
        <p className="muted">{t("settings.marks.intro")}</p>
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
        <p className="muted">
          {t("settings.keyboard.fixed", {
            first: keyLabel("Meta+1"),
            last: keyLabel("Meta+9"),
            once: approvalChord("allow_once"),
            always: approvalChord("allow_always"),
            reject: approvalChord("reject"),
          })}
        </p>

          </>,
        )}
      {page === "sessions" &&
        pageForm(
          <>
        <h3>{t("settings.defaults.title")}</h3>
        <label>
          {t("settings.defaults.provider")}
          <select
            value={form.defaults.providerId}
            onChange={(e) => {
              const p = providers.find((x) => x.id === e.target.value);
              setForm({ ...form, defaults: { ...form.defaults, providerId: e.target.value, model: p?.defaultModel ?? "" } });
            }}
          >
            <option value="">{t("settings.defaults.firstProvider")}</option>
            {providers.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          {t("settings.defaults.model")}
          <input
            value={form.defaults.model}
            placeholder={provider?.defaultModel || t("settings.defaults.modelDefault")}
            onChange={(e) => setForm({ ...form, defaults: { ...form.defaults, model: e.target.value } })}
          />
        </label>
        <label>
          {t("settings.defaults.environment")} <span className="muted">{t("settings.defaults.forProjects")}</span>
          <select
            value={form.defaults.environmentId}
            onChange={(e) => setForm({ ...form, defaults: { ...form.defaults, environmentId: e.target.value } })}
          >
            <option value="">{t("settings.defaults.local")}</option>
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
          {t("settings.defaults.profile")} <span className="muted">{t("settings.defaults.forProjects")}</span>
          <select value={form.defaults.profileId} onChange={(e) => setForm({ ...form, defaults: { ...form.defaults, profileId: e.target.value } })}>
            <option value="">{t("settings.defaults.defaultProfile")}</option>
            {profiles
              .filter((x) => x.id !== "default")
              .map((x) => (
                <option key={x.id} value={x.id}>
                  {x.name}
                </option>
              ))}
          </select>
        </label>

        <h3>{t("settings.notifications.title")}</h3>
        <p className="muted">{t("settings.notifications.intro")}</p>
        <label className="row">
          <input
            type="checkbox"
            checked={form.notifications.enabled}
            onChange={(e) => setForm({ ...form, notifications: { ...form.notifications, enabled: e.target.checked } })}
          />
          {t("settings.notifications.enabled")}
        </label>
        <label className="row">
          <input
            type="checkbox"
            disabled={!form.notifications.enabled}
            checked={form.notifications.finished}
            onChange={(e) => setForm({ ...form, notifications: { ...form.notifications, finished: e.target.checked } })}
          />
          {t("settings.notifications.finished")}
        </label>
        {projects.length > 0 && (
          <fieldset>
            <legend>
              {t("settings.notifications.projects")} <span className="muted">{t("common.savedAtOnce")}</span>
            </legend>
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
                {p.name} <span className="muted">{p.muted ? t("settings.notifications.muted") : ""}</span>
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
        <h3 id="prices">{t("settings.prices.title")}</h3>
        <PriceTable prices={form.prices ?? []} providers={providers} onChange={(prices) => setForm({ ...form, prices })} />

        <h3 id="budgets">{t("settings.budgets.title")}</h3>
        <BudgetTable budgets={form.budgets ?? []} profiles={profiles} onChange={(budgets) => setForm({ ...form, budgets })} />

            </>,
          )}
      <div className="card">
        <h3>{t("settings.usage.title")}</h3>
        <UsageTable
          version={config}
          onError={onError}
          onPrice={(providerId, model) => {
            setForm({ ...form, prices: [...(form.prices ?? []), { providerId, model, input: 0, output: 0, cacheRead: 0, cacheWrite: 0, cacheWrite1h: 0, longPromptOver: 0, from: "" }] });
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
        <h3>{t("settings.bypass.title")}</h3>
        <label className="row">
          <input
            type="checkbox"
            checked={form.permissions.allowBypass}
            onChange={(e) => allowBypass(e.target.checked)}
          />
          {t("settings.bypass.allow")}
        </label>

        <h3>{t("settings.hooks.title")}</h3>
        <HookTable hooks={form.hooks ?? []} onChange={(hooks) => setForm({ ...form, hooks })} />

            </>,
          )}
        </>
      )}
      {page === "network" &&
        pageForm(
          <>
        <h3>{t("settings.localOnly.title")}</h3>
        <label className="row">
          <input
            type="checkbox"
            checked={!!form.network?.localOnly}
            onChange={(e) => setForm({ ...form, network: { ...form.network, localOnly: e.target.checked } })}
          />
          {t("settings.localOnly.label")}
        </label>
        <p className="muted">{t("settings.localOnly.note")}</p>
        {blocked.length > 0 && (
          <details>
            <summary>{t("settings.network.blocked", { count: blocked.length })}</summary>
            <ul>
              {[...blocked].reverse().map((b, i) => (
                <li key={i}>
                  {b.host} <span className="muted">{new Date(b.at).toLocaleString(locale())}</span>
                </li>
              ))}
            </ul>
          </details>
        )}

        <h3>{t("settings.web.title")}</h3>
        <p className="muted">{tNodes("settings.web.intro", { tool: <code>{WEB_SEARCH_TOOL}</code> })}</p>
        <label>
          {t("settings.web.address")} <span className="muted">{t("settings.web.addressHint", { query: "{query}", key: "{key}" })}</span>
          <input
            value={form.web?.searchURL ?? ""}
            placeholder={SEARCH_EXAMPLE}
            onChange={(e) => setForm({ ...form, web: { ...form.web, searchURL: e.target.value } })}
          />
        </label>
        <label>
          {t("settings.web.keyHeader")} <span className="muted">{t("settings.web.keyHeaderHint", { example: "X-Subscription-Token" })}</span>
          <input value={form.web?.searchHeader ?? ""} onChange={(e) => setForm({ ...form, web: { ...form.web, searchHeader: e.target.value } })} />
        </label>
        {(form.web?.searchHeader || form.web?.searchURL?.includes("{key}")) && (
          <label>
            {t("settings.web.key")} <span className="muted">{t("settings.web.keyHint", { store: secretStore() })}</span>
            <input
              type="password"
              autoComplete="off"
              placeholder={t("settings.web.keyPlaceholder")}
              onBlur={(e) => {
                const key = e.target.value.trim();
                if (!key) return;
                e.target.value = "";
                EngineService.SetWebSearchKey(key).catch(onError);
              }}
            />
          </label>
        )}

        <h3>{t("settings.idle.title")}</h3>
        <label>
          {t("settings.idle.label")}
          <select
            value={form.sessions?.idleStop || "30m"}
            onChange={(e) => setForm({ ...form, sessions: { ...form.sessions, idleStop: e.target.value } })}
          >
            {[...IDLE_STOPS, ...(form.sessions?.idleStop && !IDLE_STOPS.includes(form.sessions.idleStop) ? [form.sessions.idleStop] : [])].map((v) => (
              <option key={v} value={v}>
                {idleStopLabel(v)}
              </option>
            ))}
          </select>
        </label>
        <p className="muted">{t("settings.idle.note")}</p>

        <h3>{t("settings.updates.title")}</h3>
        <label className="row">
          <input
            type="checkbox"
            checked={!!form.updates?.check}
            onChange={(e) => setForm({ ...form, updates: { ...form.updates, check: e.target.checked } })}
          />
          {t("settings.updates.daily")}
        </label>
        <p className="muted">{t("settings.updates.note", { version: update?.current ?? t("settings.updates.thisVersion") })}</p>
        <div className="row">
          <button type="button" disabled={checking || !!form.network?.localOnly} onClick={checkNow}>
            {checking ? t("settings.updates.checking") : t("settings.updates.checkNow")}
          </button>
          {update?.newer && update.url && (
            <span>
              {t("settings.updates.available", { version: update.latest ?? "" })}{" "}
              <a
                href={update.url}
                onClick={(e) => {
                  e.preventDefault();
                  Browser.OpenURL(update.url!).catch(onError);
                }}
              >
                {t("settings.updates.seeRelease")}
              </a>
            </span>
          )}
          {update && !update.newer && update.latest && <span className="muted">{t("settings.updates.upToDate", { version: update.current })}</span>}
          {update?.error && <span className="muted">{update.error}</span>}
        </div>

          </>,
        )}
      {page === "data" && (
        <>
      {build && (
        <div className="card build-info">
          <h3>{t("settings.build.title")}</h3>
          <p>
            <strong>{t("settings.build.version", { version: build.version })}</strong>
            {build.commit && (
              <>
                {" · "}
                <code>{build.stamp}</code>
              </>
            )}
          </p>
          <p className="muted">{build.commit ? t("settings.build.intro") : t("settings.build.unknown")}</p>
          {build.dirty && <p className="muted">{t("settings.build.dirty")}</p>}
        </div>
      )}
      <div className="card">
        <h3>{t("settings.import.title")}</h3>
        <p className="muted">{t("settings.import.intro")}</p>
        <div className="row">
          <button type="button" onClick={onImport}>
            {t("settings.import.button")}
          </button>
        </div>

      </div>
      <div className="card">
        <h3>{t("settings.backup.title")}</h3>
        <p className="muted">{t("settings.backup.intro", { store: secretStore() })}</p>
        <div className="row">
          <button type="button" onClick={backUp}>
            {t("settings.backup.export")}
          </button>
          <button type="button" onClick={chooseBackup}>
            {t("settings.backup.restore")}
          </button>
          {backup && <span className="ok">{t("settings.backup.savedTo", { path: backup })}</span>}
        </div>
        {preview && <RestorePreview preview={preview} onRestore={() => restore(preview)} onCancel={() => setPreview(null)} />}
        {restored && (
          <div className="banner" role="status">
            {t("settings.restored.files", { count: restored.files?.length ?? 0 })} {restored.backupNote}
            {restored.needApproval > 0 && <p>{t("settings.restored.approval")}</p>}
            {(restored.gone ?? []).length > 0 && <p>{t("settings.restored.gone", { list: (restored.gone ?? []).join("; ") })}</p>}
            {(restored.needKeys ?? []).length > 0 && (
              <p>
                {tNodes("settings.restored.needKeys", {
                  providers: list(restored.needKeys ?? []),
                  link: (
                    <button type="button" className="link" onClick={onProviders}>
                      {t("settings.page.providers")}
                    </button>
                  ),
                })}
              </p>
            )}
          </div>
        )}
      </div>

      <div className="card">
        <h3>{t("settings.report.title")}</h3>
        <p className="muted">
          {reportSession ? t("settings.report.introNamed", { title: reportSession.title }) : t("settings.report.introLast")} {t("settings.report.privacy")}
        </p>
        <div className="row">
          <button type="button" onClick={copyReport}>
            {t("settings.report.copy")}
          </button>
          <button type="button" onClick={toggleReport} aria-expanded={report !== null}>
            {report !== null ? t("settings.report.hide") : t("settings.report.show")}
          </button>
          <button type="button" onClick={() => EngineService.ShowLogs().catch(onError)}>
            {t("settings.report.showLogs", { where: inFileManager() })}
          </button>
          {reportCopied && (
            <span className="ok" role="status">
              {t("settings.report.copied")}
            </span>
          )}
        </div>
        {report !== null && (
          <pre className="report-preview" tabIndex={0} aria-label={t("settings.report.label")}>
            {report}
          </pre>
        )}
      </div>

        </>
      )}
    </section>
  );
}
