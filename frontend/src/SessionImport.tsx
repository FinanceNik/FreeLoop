import { useMemo, useState } from "react";
import { EngineService, type AgentSession, type ProjectInfo, type Profile } from "./api.ts";
import { canImport, folderName, importResult, matchesImport } from "./sessionimport.ts";
import { locale, t } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";

// SessionImport brings an external agent's own sessions here (SES-020): the agent
// lists them and copies the chosen ones, and each continues as a session here. It
// offers any agent the same way (HRN-001); one that cannot list says so.

interface Props {
  profiles: Profile[];
  projects: ProjectInfo[];
  onOpen: (sid: string) => void;
  onImported: () => void;
  onError: (err: unknown) => void;
}

export default function SessionImport({ profiles, projects, onOpen, onImported, onError }: Props) {
  const agents = useMemo(() => profiles.filter((p) => p.harness && p.harness !== "builtin"), [profiles]);
  const [profileId, setProfileId] = useState("");
  const agent = profileId || (agents.find((a) => a.id === "claude-code") ?? agents[0])?.id || "";
  const [folder, setFolder] = useState("");
  const [extraFolder, setExtraFolder] = useState("");
  const [rows, setRows] = useState<AgentSession[] | null>(null);
  const [searching, setSearching] = useState(false);
  const [query, setQuery] = useState("");
  const [picked, setPicked] = useState<string[]>([]);
  const [progress, setProgress] = useState<{ done: number; total: number } | null>(null);
  const [result, setResult] = useState<{ done: number; failed: number } | null>(null);
  const [imported, setImported] = useState<Record<string, string>>({});

  const find = (profile = agent, where = folder) => {
    setSearching(true);
    setResult(null);
    setPicked([]);
    setImported({});
    EngineService.AgentSessions(profile, where)
      .then((list) => setRows(list ?? []))
      .catch((err) => {
        setRows(null);
        onError(err);
      })
      .finally(() => setSearching(false));
  };

  const chooseFolder = () =>
    EngineService.ChooseFolder()
      .then((p) => {
        if (!p) return;
        setExtraFolder(p);
        setFolder(p);
        find(agent, p);
      })
      .catch(onError);

  const shown = useMemo(() => (rows ?? []).map((r) => ({ ...r, sessionId: imported[r.id] ?? r.sessionId })).filter((r) => matchesImport(r, query)), [rows, imported, query]);
  const pickable = shown.filter(canImport);

  const run = async () => {
    const chosen = (rows ?? []).filter((r) => picked.includes(r.id));
    let done = 0;
    let failed = 0;
    setResult(null);
    setProgress({ done: 0, total: chosen.length });
    for (const r of chosen) {
      try {
        const ses = await EngineService.ImportAgentSession({ profileId: agent, folder: r.folder, id: r.id, title: r.title });
        done++;
        if (ses) setImported((m) => ({ ...m, [r.id]: ses.id }));
      } catch (err) {
        failed++;
        onError(err);
      }
      setProgress({ done: done + failed, total: chosen.length });
    }
    setProgress(null);
    setPicked([]);
    setResult({ done, failed });
    onImported();
  };

  if (agents.length === 0) return <p className="muted">{t("sessionImport.noAgents")}</p>;
  const busy = searching || progress !== null;
  return (
    <div className="session-import">
      <p className="muted">{t("sessionImport.intro")}</p>
      <div className="row session-import-where">
        {agents.length > 1 && (
          <label>
            {t("sessionImport.agent")}{" "}
            <select
              value={agent}
              disabled={busy}
              onChange={(e) => {
                setProfileId(e.target.value);
                setRows(null);
              }}
            >
              {agents.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </select>
          </label>
        )}
        <label>
          {t("sessionImport.folder")}{" "}
          <select
            value={folder}
            disabled={busy}
            onChange={(e) => {
              setFolder(e.target.value);
              if (rows) find(agent, e.target.value);
            }}
          >
            <option value="">{t("sessionImport.allFolders")}</option>
            {projects.map((p) => (
              <option key={p.id} value={p.root}>
                {folderName(p.root)}
              </option>
            ))}
            {extraFolder && !projects.some((p) => p.root === extraFolder) && <option value={extraFolder}>{folderName(extraFolder)}</option>}
          </select>
        </label>
        <button disabled={busy} onClick={chooseFolder}>
          {t("sessionImport.chooseFolder")}
        </button>
        <button className={rows ? "" : "primary"} disabled={busy} onClick={() => find()}>
          {searching ? t("sessionImport.looking") : rows ? t("sessionImport.lookAgain") : t("sessionImport.find")}
        </button>
      </div>
      {searching && !rows && <p className="muted">{t("sessionImport.starting")}</p>}
      {rows && rows.length === 0 && <p className="muted">{t("sessionImport.none")}</p>}
      {rows && rows.length > 0 && (
        <>
          <div className="row session-import-where">
            <input type="search" aria-label={t("sessionImport.filterLabel")} placeholder={t("sessionImport.filter")} value={query} onChange={(e) => setQuery(e.target.value)} />
            <button disabled={busy || pickable.length === 0} onClick={() => setPicked(picked.length === pickable.length ? [] : pickable.map((r) => r.id))}>
              {picked.length === pickable.length && pickable.length > 0 ? t("sessionImport.selectNone") : t("sessionImport.selectAll")}
            </button>
          </div>
          <ul className="import-list session-import-list">
            {shown.map((r) => (
              <li key={r.id}>
                <label className="row">
                  <input
                    type="checkbox"
                    disabled={busy || !canImport(r)}
                    checked={picked.includes(r.id)}
                    onChange={(e) => setPicked((p) => (e.target.checked ? [...p, r.id] : p.filter((k) => k !== r.id)))}
                  />
                  <strong>{r.title || t("sessionImport.untitled")}</strong>
                  {r.missing && <span className="badge">{t("sessionImport.folderGone")}</span>}
                </label>
                <div className="import-detail muted">
                  <span title={r.folder}>{folderName(r.folder)}</span>
                  {r.updatedAt && !r.updatedAt.startsWith("0001") && ` · ${new Date(r.updatedAt).toLocaleString(locale())}`}
                  {r.sessionId && (
                    <>
                      {" · "}
                      {tNodes("sessionImport.hereAlready", {
                        open: (
                          <button className="link" onClick={() => onOpen(r.sessionId)}>
                            {t("sessionImport.open")}
                          </button>
                        ),
                      })}
                    </>
                  )}
                </div>
              </li>
            ))}
          </ul>
        </>
      )}
      {result && <p className="ok">{importResult(result.done, result.failed)}</p>}
      {rows && rows.length > 0 && (
        <div className="row">
          <button className="primary" disabled={busy || picked.length === 0} onClick={run}>
            {progress ? t("sessionImport.importing", { current: progress.done + 1, total: progress.total }) : t("sessionImport.importSelected", { count: picked.length })}
          </button>
        </div>
      )}
    </div>
  );
}
