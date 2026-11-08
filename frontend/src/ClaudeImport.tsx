import { useEffect, useState } from "react";
import { secretStore } from "./platform.ts";
import { EngineService, type ClaudeImport as Preview, type SkillCandidate } from "./api.ts";

// ClaudeImport is the one-way import from Claude Code (MCP-002, SKL-003,
// SET-003): MCP servers from ~/.claude.json, permission rules from
// ~/.claude/settings.json and skills from ~/.claude/skills, each listed
// with where it goes. Nothing is written back to Claude Code's files.

interface Props {
  onDone?: () => void;
  onError: (err: unknown) => void;
}

const STATUS: Record<string, string> = { new: "new", same: "already here", changed: "replaces", unsupported: "not imported", invalid: "cannot load" };

export default function ClaudeImport({ onDone, onError }: Props) {
  const [preview, setPreview] = useState<Preview | null>(null);
  const [skills, setSkills] = useState<{ dir: string; cands: SkillCandidate[] } | null>(null);
  const [picked, setPicked] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState("");

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
      setResult(`Imported ${n} ${n === 1 ? "item" : "items"} and ${done.length} ${done.length === 1 ? "skill" : "skills"}. Imported MCP servers run only after you approve them on the MCP page.`);
      load();
      onDone?.();
    } catch (err) {
      onError(err);
    } finally {
      setBusy(false);
    }
  };

  if (!preview) return <p className="muted">Reading Claude Code's settings…</p>;
  const items = preview.items ?? [];
  const groups = [
    { title: "MCP servers", items: items.filter((i) => i.kind === "mcp") },
    { title: "Permission rules", items: items.filter((i) => i.kind === "rule") },
  ];
  const skillCands = skills?.cands ?? [];
  return (
    <div className="claude-import">
      <p className="muted">
        Read from {[...(preview.sources ?? []), ...(skillCands.length > 0 && skills ? [skills.dir] : [])].map((s) => s).join(", ") || "nothing"}
        {(preview.missing ?? []).length > 0 && `; not found: ${(preview.missing ?? []).join(", ")}`}. Nothing is written to Claude Code's files.
      </p>
      {groups.map(
        (g) =>
          g.items.length > 0 && (
            <section key={g.title}>
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
                      <strong>{it.label}</strong> <span className={`badge import-${it.status}`}>{STATUS[it.status] ?? it.status}</span>
                    </label>
                    <div className="import-detail">
                      {it.kind === "mcp" && <code>{it.detail}</code>}
                      {it.destination !== "none" && <div className="muted">→ {it.destination}</div>}
                      {(it.secrets ?? []).length > 0 && <div className="muted">Moved to {secretStore()}: {(it.secrets ?? []).join(", ")}</div>}
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
          <h3>Skills</h3>
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
                  <strong>{c.name}</strong> <span className={`badge import-${c.status}`}>{STATUS[c.status] ?? c.status}</span>
                </label>
                <div className="import-detail muted">
                  → the skill store{c.issue ? ` · ${c.issue}` : ""}
                  {(c.scripts ?? []).length > 0 && ` · scripts: ${(c.scripts ?? []).join(", ")}`}
                </div>
              </li>
            ))}
          </ul>
        </section>
      )}
      {items.length === 0 && skillCands.length === 0 && <p className="muted">Claude Code has nothing to import here.</p>}
      {result && <p className="ok">{result}</p>}
      <div className="row">
        <button className="primary" disabled={busy || picked.length === 0} onClick={apply}>
          Import {picked.length} selected
        </button>
      </div>
    </div>
  );
}
