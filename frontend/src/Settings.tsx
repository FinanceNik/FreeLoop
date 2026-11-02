import { useEffect, useState, type FormEvent } from "react";
import ShortcutsEditor from "./ShortcutsEditor.tsx";
import ColorEditor from "./ColorEditor.tsx";
import type { ColorSet } from "./colors.ts";
import { bindingOf, keyLabel } from "./shortcuts.ts";
import { Browser } from "@wailsio/runtime";
import { PriceTable, UsageTable } from "./Costs.tsx";
import { HookTable, PendingCommands } from "./Hooks.tsx";
import { EngineService, type Blocked, type ConfigRestore, type Config, type UpdateInfo, type EnvironmentInfo, type Profile, type ProjectInfo, type ProviderInfo } from "./api.ts";

interface Props {
  config: Config;
  providers: ProviderInfo[];
  environments: EnvironmentInfo[];
  profiles: Profile[];
  projects: ProjectInfo[];
  onProjectsChanged: () => void;
  onImport: () => void;
  onProviders: () => void;
  colors: ColorSet;
  onColors: (c: ColorSet) => void;
  onSaved: (c: Config) => void;
  onError: (err: unknown) => void;
}

export default function Settings({ config, providers, environments, profiles, projects, onProjectsChanged, onImport, onProviders, colors, onColors, onSaved, onError }: Props) {
  const [form, setForm] = useState<Config>(config);
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
  const backUp = () =>
    EngineService.ExportConfig()
      .then((path) => path && setBackup(path))
      .catch(onError);
  const restore = () => {
    if (!confirm("Restore a config backup? Its settings, colors, providers, profiles, environments, token optimizers, rules, MCP servers, hooks and agent presets replace the current ones; the replaced files are kept in the data folder's backups folder. Hooks and agents it brings run nothing until you approve them here.")) return;
    EngineService.ImportConfig()
      .then((r) => r && setRestored(r))
      .catch(onError);
  };
  // ⌘+ and ⌘- change one field; the rest of the form may hold unsaved edits.
  // Approving a command is not an edit either.
  const { ui: { fontSize: savedFontSize, ...savedUi }, commandApprovals: _approvals, ...savedRest } = config;
  useEffect(() => setForm(config), [JSON.stringify([savedRest, savedUi])]);
  useEffect(() => setForm((f) => ({ ...f, ui: { ...f.ui, fontSize: savedFontSize } })), [savedFontSize]);
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
    if (form.permissions.allowBypass && !config.permissions.allowBypass) {
      const ok = confirm(
        "Allow bypass mode? In bypass mode tool calls run without asking. Protected paths and deny rules still hold, but shell commands can do anything you can. New sessions never start in bypass.",
      );
      if (!ok) return;
    }
    EngineService.SaveSettings(form)
      .then((c) => {
        onSaved(c);
        setSaved(true);
        setTimeout(() => setSaved(false), 1500);
      })
      .catch(onError);
  };

  const provider = providers.find((p) => p.id === form.defaults.providerId);

  return (
    <section className="page">
      <h2>General</h2>
      <form className="card form" onSubmit={save}>
        <h3>Appearance</h3>
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

        <h3>Colors</h3>
        <ColorEditor colors={colors} onChange={onColors} onError={onError} />

        <h3>Keyboard shortcuts</h3>
        <ShortcutsEditor value={form.ui.shortcuts ?? {}} onChange={(shortcuts) => setForm({ ...form, ui: { ...form.ui, shortcuts } })} />
        <p className="muted">⌘1 to ⌘9 open the sessions listed first, and ⌘↩, ⌘⇧↩ and ⌘⇧⌫ answer an approval from anywhere; those stay as they are.</p>

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

        <h3>Network</h3>
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

        <h3>Permissions</h3>
        <label className="row">
          <input
            type="checkbox"
            checked={form.permissions.allowBypass}
            onChange={(e) => setForm({ ...form, permissions: { ...form.permissions, allowBypass: e.target.checked } })}
          />
          Allow bypass mode to be chosen for a session
        </label>

        <h3>Claude Code</h3>
        <p className="muted">
          Copy MCP servers, permission rules and skills from Claude Code. <button type="button" className="link" onClick={onImport}>Import from Claude Code…</button>
        </p>

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

        <h3>Hooks</h3>
        <HookTable hooks={form.hooks ?? []} onChange={(hooks) => setForm({ ...form, hooks })} />

        <h3 id="prices">Prices</h3>
        <PriceTable prices={form.prices ?? []} providers={providers} onChange={(prices) => setForm({ ...form, prices })} />

        <div className="row">
          <button type="submit" className="primary">
            Save
          </button>
          {saved && <span className="ok">Saved</span>}
        </div>
      </form>

      <PendingCommands version={[config, restored]} onSaved={onSaved} onError={onError} />

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
          <button type="button" onClick={restore}>
            Restore from a backup…
          </button>
          {backup && <span className="ok">Saved to {backup}</span>}
        </div>
        {restored && (
          <div className="banner" role="status">
            Restored {restored.files?.length ?? 0} files. {restored.backupNote}
            {restored.needApproval > 0 && <p>Hooks and agents waiting for your approval, listed above, run nothing until you approve them.</p>}
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
        <h3>Usage and estimated cost</h3>
        <UsageTable
          version={config}
          onError={onError}
          onPrice={(providerId, model) => {
            setForm({ ...form, prices: [...(form.prices ?? []), { providerId, model, input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }] });
            document.getElementById("prices")?.scrollIntoView({ block: "start" });
          }}
        />
      </div>
    </section>
  );
}
