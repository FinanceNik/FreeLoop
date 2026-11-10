import { useEffect, useState } from "react";
import { secretStore } from "./platform.ts";
import { EngineService, type ClaudeImport as Preview, type SkillCandidate } from "./api.ts";
import { t, type PlainKey } from "./i18n.ts";

// ClaudeImport is the one-way import from Claude Code (MCP-002, SKL-003,
// SET-003): MCP servers from ~/.claude.json, permission rules from
// ~/.claude/settings.json and skills from ~/.claude/skills, each listed
// with where it goes. Nothing is written back to Claude Code's files.

interface Props {
  onDone?: () => void;
  onError: (err: unknown) => void;
}

// What an item will do, as a catalogue key; a status not listed shows as the engine names it.
const STATUS: Record<string, PlainKey> = {
  new: "claudeImport.status.new",
  same: "claudeImport.status.same",
  changed: "claudeImport.status.changed",
  unsupported: "claudeImport.status.unsupported",
  invalid: "claudeImport.status.invalid",
};
function statusName(status: string): string {
  const key = STATUS[status];
  return key ? t(key) : status;
}

export default function ClaudeImport({ onDone, onError }: Props) {
  const [preview, setPreview] = useState<Preview | null>(null);
  const [skills, setSkills] = useState<{ dir: string; cands: SkillCandidate[] } | null>(null);
  const [picked, setPicked] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ items: number; skills: number } | null>(null);

  const load = () => {
    EngineService.PreviewClaudeImport()
      .then((p) => {
        setPreview(p);
        const keys = (p.items ?? []).filter((i) => i.status === "new" || i.status === "changed").map((i) => i.key);
        setPicked((old) => [...old.filter((k) => k.startsWith("skill:")), ...keys]);
      })
      .catch(onError);
    EngineService.ClaudeSkillsDir()
      .then((dir) =>
        EngineService.PreviewSkills(dir)
          .then((cands) => {
            setSkills({ dir, cands: cands ?? [] });
            const keys = (cands ?? []).filter((c) => c.status === "new" || c.status === "changed").map((c) => "skill:" + c.name);
            setPicked((old) => [...old.filter((k) => !k.startsWith("skill:")), ...keys]);
          })
          .catch(() => setSkills({ dir, cands: [] })),
      )
      .catch(onError);
  };
  useEffect(load, []);

  const toggle = (key: string, on: boolean) => setPicked((p) => (on ? [...p, key] : p.filter((k) => k !== key)));

  const apply = async () => {
    setBusy(true);
    try {
      const n = await EngineService.ApplyClaudeImport(picked.filter((k) => !k.startsWith("skill:")));
      const names = picked.filter((k) => k.startsWith("skill:")).map((k) => k.slice(6));
      const done = names.length > 0 && skills ? ((await EngineService.InstallSkills(skills.dir, names)) ?? []) : [];
      setResult({ items: n, skills: done.length });
      load();
      onDone?.();
    } catch (err) {
      onError(err);
    } finally {
      setBusy(false);
    }
  };

  if (!preview) return <p className="muted">{t("claudeImport.reading")}</p>;
  const items = preview.items ?? [];
  const skillCands = skills?.cands ?? [];
  const groups = [
    { id: "mcp", title: t("settings.page.mcp"), items: items.filter((i) => i.kind === "mcp") },
    { id: "rule", title: t("claudeImport.group.rules"), items: items.filter((i) => i.kind === "rule") },
  ];
  const sources = [...(preview.sources ?? []), ...(skillCands.length > 0 && skills ? [skills.dir] : [])].join(", ") || t("claudeImport.nothing");
  const missing = (preview.missing ?? []).join(", ");
  return (
    <div className="claude-import">
      <p className="muted">
        {missing ? t("claudeImport.readFromMissing", { sources, missing }) : t("claudeImport.readFrom", { sources })} {t("claudeImport.notWritten")}
      </p>
      {groups.map(
        (g) =>
          g.items.length > 0 && (
            <section key={g.id}>
              <h3>{g.title}</h3>
              <ul className="import-list">
                {g.items.map((it) => (
                  <li key={it.key}>
                    <label className="row">
                      <input
                        type="checkbox"
                        disabled={it.status === "same" || it.status === "unsupported"}
                        checked={picked.includes(it.key)}
                        onChange={(e) => toggle(it.key, e.target.checked)}
                      />
                      <strong>{it.label}</strong> <span className={`badge import-${it.status}`}>{statusName(it.status)}</span>
                    </label>
                    <div className="import-detail">
                      {it.kind === "mcp" && <code>{it.detail}</code>}
                      {it.destination !== "none" && <div className="muted">→ {it.destination}</div>}
                      {(it.secrets ?? []).length > 0 && <div className="muted">{t("claudeImport.movedTo", { store: secretStore(), secrets: (it.secrets ?? []).join(", ") })}</div>}
                      {it.note && <div className="muted">{it.note}</div>}
                    </div>
                  </li>
                ))}
              </ul>
            </section>
          ),
      )}
      {skillCands.length > 0 && (
        <section>
          <h3>{t("settings.page.skills")}</h3>
          <ul className="import-list">
            {skillCands.map((c) => (
              <li key={c.name}>
                <label className="row">
                  <input
                    type="checkbox"
                    disabled={c.status === "same" || c.status === "invalid"}
                    checked={picked.includes("skill:" + c.name)}
                    onChange={(e) => toggle("skill:" + c.name, e.target.checked)}
                  />
                  <strong>{c.name}</strong> <span className={`badge import-${c.status}`}>{statusName(c.status)}</span>
                </label>
                <div className="import-detail muted">
                  {[t("claudeImport.skillStore"), c.issue, (c.scripts ?? []).length > 0 && t("claudeImport.scripts", { scripts: (c.scripts ?? []).join(", ") })]
                    .filter(Boolean)
                    .join(" · ")}
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}
      {items.length === 0 && skillCands.length === 0 && <p className="muted">{t("claudeImport.nothingToImport")}</p>}
      {result && <p className="ok">{t("claudeImport.result", { items: t("claudeImport.items", { count: result.items }), count: result.skills })} {t("claudeImport.approveMcp")}</p>}
      <div className="row">
        <button className="primary" disabled={busy || picked.length === 0} onClick={apply}>
          {t("claudeImport.importSelected", { count: picked.length })}
        </button>
      </div>
    </div>
  );
}
