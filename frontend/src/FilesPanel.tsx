import { useEffect, useMemo, useState, type ReactElement } from "react";
import { EngineService, type FileEntry, type FileView, type Session } from "./api.ts";
import type { FileChange } from "./transcript.ts";

// FilesPanel is the Files tab of the right panel (GUI-013): a read-only
// tree of the session's folder with the files the agent edited, and those
// git reports as changed, marked; a file opens in the viewer, or in the
// user's own editor.

interface Props {
  session: Session;
  // Changes the git state event, so the changed marks refresh.
  gitKey: string;
  agentEdits: FileChange[];
  onError: (err: unknown) => void;
}

function relative(path: string, root: string): string {
  const r = root.endsWith("/") ? root : root + "/";
  if (path.startsWith(r)) return path.slice(r.length);
  return path.replace(/^\.\//, "");
}

export default function FilesPanel({ session, gitKey, agentEdits, onError }: Props) {
  const root = session.worktree || session.projectRoot;
  const [folders, setFolders] = useState<Record<string, FileEntry[]>>({});
  const [open, setOpen] = useState<Set<string>>(new Set([""]));
  const [view, setView] = useState<FileView | null>(null);
  const [gitChanged, setGitChanged] = useState<string[]>([]);

  const load = (rel: string) =>
    EngineService.ListFolder(session.id, rel)
      .then((list) => setFolders((f) => ({ ...f, [rel]: list ?? [] })))
      .catch(onError);
  // The open folders reload when the agent changes something; closed ones
  // are forgotten, so they load afresh when opened again.
  useEffect(() => {
    setFolders((f) => Object.fromEntries(Object.entries(f).filter(([rel]) => open.has(rel))));
    for (const rel of open) load(rel);
    EngineService.Changes(session.id)
      .then((c) => setGitChanged(c.repo && !c.blocked ? (c.files ?? []).map((f) => f.path) : []))
      .catch(() => setGitChanged([]));
  }, [session.id, gitKey]);

  const edited = useMemo(() => new Set(agentEdits.map((c) => relative(c.path, root))), [agentEdits, root]);
  const changed = useMemo(() => new Set(gitChanged), [gitChanged]);
  // A folder is marked when something inside it changed.
  const marked = (path: string, dir: boolean) => {
    if (!dir) return edited.has(path) ? "agent" : changed.has(path) ? "git" : "";
    const prefix = path + "/";
    for (const p of edited) if (p.startsWith(prefix)) return "agent";
    for (const p of changed) if (p.startsWith(prefix)) return "git";
    return "";
  };

  const toggle = (rel: string) => {
    const next = new Set(open);
    if (next.has(rel)) next.delete(rel);
    else {
      next.add(rel);
      if (!folders[rel]) load(rel);
    }
    setOpen(next);
  };
  const show = (rel: string) =>
    EngineService.ViewFile(session.id, rel)
      .then(setView)
      .catch(onError);

  const tree = (rel: string, depth: number): ReactElement[] =>
    (folders[rel] ?? []).flatMap((f) => {
      const mark = marked(f.path, f.isDir);
      const row = (
        <li key={f.path}>
          <button
            className={`file-row ${mark ? `file-${mark}` : ""} ${view?.path === f.path ? "file-on" : ""}`}
            style={{ paddingLeft: 6 + depth * 14 }}
            title={mark === "agent" ? "Changed by the agent" : mark === "git" ? "Changed since the last commit" : f.path}
            aria-expanded={f.isDir ? open.has(f.path) : undefined}
            onClick={() => (f.isDir ? toggle(f.path) : show(f.path))}
          >
            <span className="file-icon">{f.isDir ? (open.has(f.path) ? "▾" : "▸") : ""}</span>
            {f.name}
            {mark && <span className="file-mark" aria-label={mark === "agent" ? "changed by the agent" : "changed"} />}
          </button>
        </li>
      );
      return f.isDir && open.has(f.path) ? [row, ...tree(f.path, depth + 1)] : [row];
    });

  return (
    <div className="files-panel">
      <ul className="file-tree" aria-label="Files">
        {tree("", 0)}
        {folders[""]?.length === 0 && <li className="muted">The folder is empty.</li>}
      </ul>
      <div className="file-view">
        {!view && <p className="muted">Choose a file. Files the agent changed are marked.</p>}
        {view && (
          <>
            <div className="row file-view-head">
              <code>{view.path}</code>
              <button className="link" onClick={() => EngineService.OpenFile(session.id, view.path).catch(onError)}>
                Open in editor
              </button>
            </div>
            {view.binary ? (
              <p className="muted">A binary file ({view.size.toLocaleString()} bytes).</p>
            ) : (
              <pre className="file-text">
                {view.text.replace(/\n$/, "").split("\n").map((line, i) => (
                  <div key={i}>
                    <span className="file-line" aria-hidden="true">
                      {i + 1}
                    </span>
                    {line}
                  </div>
                ))}
              </pre>
            )}
            {view.truncated && <p className="muted">Only the first 512 KB are shown.</p>}
          </>
        )}
      </div>
    </div>
  );
}
