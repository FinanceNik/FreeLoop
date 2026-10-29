import { useEffect, useState, type FormEvent } from "react";
import { EngineService, type Config, type EnvironmentInfo, type Profile, type ProjectInfo, type ProviderInfo } from "./api.ts";

interface Props {
  config: Config;
  providers: ProviderInfo[];
  environments: EnvironmentInfo[];
  profiles: Profile[];
  projects: ProjectInfo[];
  onProjectsChanged: () => void;
  onImport: () => void;
  onSaved: (c: Config) => void;
  onError: (err: unknown) => void;
}

export default function Settings({ config, providers, environments, profiles, projects, onProjectsChanged, onImport, onSaved, onError }: Props) {
  const [form, setForm] = useState<Config>(config);
  const [saved, setSaved] = useState(false);
  useEffect(() => setForm(config), [config]);

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
      <h2>Settings</h2>
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
        </label>

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

        <div className="row">
          <button type="submit" className="primary">
            Save
          </button>
          {saved && <span className="ok">Saved</span>}
        </div>
      </form>
    </section>
  );
}
