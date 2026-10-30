import { useEffect, useRef, useState } from "react";
import { EngineService, type ChangedFile, type Changes, type FileDiff, type Session } from "./api.ts";
import { diffStats, parseDiff } from "./diff.ts";
import type { FileChange } from "./transcript.ts";
import { showTool } from "./SessionPanel.tsx";
import { usePaletteActions } from "./Palette.tsx";
import PullRequest from "./PullRequest.tsx";
import type { PaletteItem } from "./palette.ts";

// DiffPanel is the Diff tab of the right panel (GUI-005): the files that
// differ from the session's base commit, each with its diff, plus staging,
// committing and discarding. It refetches whenever the session's git state
// changes, which the engine records after every tool call, turn and panel
// action.

interface Props {
  session: Session;
  // Changes the git state event, so the list refetches.
  gitKey: string;
  // Files the agent's file tools edited, to mark them and jump to the edit.
  agentEdits: FileChange[];
  onError: (err: unknown) => void;
}

const STATUS_MARK: Record<string, string> = { added: "A", modified: "M", deleted: "D", untracked: "U" };

export default function DiffPanel({ session, gitKey, agentEdits, onError }: Props) {
  const [changes, setChanges] = useState<Changes | null>(null);
  const [open, setOpen] = useState<string | null>(null);
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [suggesting, setSuggesting] = useState(false);
  const [committed, setCommitted] = useState("");
  // Merging a worktree session's branch back (SES-010).
  const [merging, setMerging] = useState(false);
  const [merged, setMerged] = useState("");
  // Bumped after each action so an open diff refetches.
  const [version, setVersion] = useState(0);

  const load = () =>
    EngineService.Changes(session.id)
      .then((c) => {
        setChanges(c);
        setVersion((v) => v + 1);
      })
      .catch(onError);
  useEffect(() => {
    load();
  }, [session.id, gitKey]);

  const act = (p: Promise<unknown>) => {
    setBusy(true);
    return p
      .catch(onError)
      .finally(() => {
        setBusy(false);
        load();
      });
  };

  // The panel's actions in the palette (GUI-014), while the tab is open.
  const messageBox = useRef<HTMLTextAreaElement>(null);
  const live = changes?.repo && !changes.blocked ? (changes.files ?? []) : [];
  const liveStaged = live.filter((f) => f.staged);
  const liveUncommitted = live.filter((f) => f.staged || f.unstaged);
  const item = (id: string, title: string, run: () => void, extra: Partial<PaletteItem> = {}): PaletteItem => ({
    id: `diff:${session.id}:${id}`,
    title,
    group: "Changes",
    run,
    ...extra,
  });
  usePaletteActions(
    changes?.repo && !changes.blocked
      ? [
          item("refresh", "Refresh changes", load),
          ...(liveUncommitted.some((f) => f.unstaged) ? [item("stage", "Stage all changes", () => act(EngineService.StageFiles(session.id, liveUncommitted.map((f) => f.path), true)))] : []),
          ...(liveStaged.length ? [item("unstage", "Unstage all changes", () => act(EngineService.StageFiles(session.id, liveStaged.map((f) => f.path), false)))] : []),
          ...(liveStaged.length ? [item("commit", `Commit ${liveStaged.length} staged…`, () => messageBox.current?.focus(), { detail: "write the message, then Commit" })] : []),
          ...(liveUncommitted.length ? [item("suggest", "Suggest a commit message", () => suggest(), { detail: "uses tokens" })] : []),
          ...(session.worktree ? [item("merge", "Merge into the main checkout…", () => setMerging(true), { keywords: "worktree branch" })] : []),
          ...live.flatMap((f) => [
            item(`open:${f.path}`, `Show the diff of ${f.path}`, () => setOpen(f.path), { keywords: "file" }),
            ...(f.unstaged ? [item(`stage:${f.path}`, `Stage ${f.path}`, () => act(EngineService.StageFiles(session.id, [f.path], true)))] : []),
            ...(f.staged ? [item(`unstage:${f.path}`, `Unstage ${f.path}`, () => act(EngineService.StageFiles(session.id, [f.path], false)))] : []),
            ...(f.staged || f.unstaged
              ? [
                  item(`discard:${f.path}`, `Discard changes to ${f.path}…`, () => {
                    if (confirm(`Discard the changes to ${f.path}? This cannot be undone.`)) act(EngineService.DiscardFile(session.id, f.path));
                  }),
                ]
              : []),
          ]),
        ]
      : [],
  );

  if (!changes) return <p className="muted">Loading…</p>;
  if (!changes.repo) return <p className="muted">The project is not a git repository.</p>;
  if (changes.blocked) return <p className="banner warn">{changes.blocked}</p>;

  const files = changes.files ?? [];
  const staged = files.filter((f) => f.staged);
  const uncommitted = files.filter((f) => f.staged || f.unstaged);
  const edited = new Map(agentEdits.map((c) => [relative(c.path, session.projectRoot), c]));

  const commit = () =>
    act(
      EngineService.Commit(session.id, message).then((hash) => {
        setCommitted(hash);
        setMessage("");
      }),
    );
  const suggest = () => {
    setSuggesting(true);
    EngineService.SuggestCommitMessage(session.id)
      .then((m) => setMessage(m))
      .catch(onError)
      .finally(() => setSuggesting(false));
  };

  return (
    <div className="diff-panel">
      <div className="row diff-head">
        <span className="muted">
          {files.length} {files.length === 1 ? "file" : "files"} changed since{" "}
          {changes.base ? <code title={changes.base}>{changes.base.slice(0, 8)}</code> : "the first commit"}
        </span>
        <button className="link" onClick={load} disabled={busy}>
          Refresh
        </button>
      </div>
      {session.worktree && (
        <div className="worktree-bar">
          <span className="muted">
            Own worktree on <code>{session.branch}</code>
          </span>
          {merging ? (
            <span className="row">
              <button
                className="primary"
                disabled={busy}
                onClick={() =>
                  act(
                    EngineService.MergeWorktree(session.id).then((r) => {
                      setMerged(r.message);
                      setMerging(false);
                    }),
                  )
                }
              >
                Merge into the main checkout
              </button>
              <button onClick={() => setMerging(false)}>Cancel</button>
            </span>
          ) : (
            <button onClick={() => setMerging(true)} title="Merges the session's committed work into the branch the project's main checkout is on">
              Merge…
            </button>
          )}
        </div>
      )}
      {merged && <p className="ok">{merged}</p>}
      {files.length === 0 && <p className="muted">No changes against the session's base commit.</p>}
      {uncommitted.length > 0 && (
        <div className="row">
          <button
            disabled={busy || uncommitted.every((f) => f.staged && !f.unstaged)}
            onClick={() => act(EngineService.StageFiles(session.id, uncommitted.map((f) => f.path), true))}
          >
            Stage all
          </button>
          <button disabled={busy || staged.length === 0} onClick={() => act(EngineService.StageFiles(session.id, staged.map((f) => f.path), false))}>
            Unstage all
          </button>
        </div>
      )}
      <ul className="panel-list diff-files">
        {files.map((f) => (
          <FileRow
            key={f.path}
            file={f}
            sessionId={session.id}
            open={open === f.path}
            version={version}
            edit={edited.get(f.path)}
            busy={busy}
            onToggle={() => setOpen(open === f.path ? null : f.path)}
            onStage={(on) => act(EngineService.StageFiles(session.id, [f.path], on))}
            onDiscard={() => act(EngineService.DiscardFile(session.id, f.path))}
            onError={onError}
          />
        ))}
      </ul>

      <h3>Commit</h3>
      <textarea
        className="commit-message"
        ref={messageBox}
        rows={4}
        placeholder="Commit message"
        aria-label="Commit message"
        value={message}
        onChange={(e) => setMessage(e.target.value)}
      />
      <div className="row">
        <button className="primary" disabled={busy || staged.length === 0 || !message.trim()} onClick={commit}>
          Commit {staged.length} staged
        </button>
        <button disabled={suggesting || uncommitted.length === 0} onClick={suggest} title="Asks the session's model; uses tokens">
          {suggesting ? "Suggesting…" : "Suggest message"}
        </button>
      </div>
      {committed && (
        <p className="muted">
          Committed <code>{committed.slice(0, 8)}</code>.
        </p>
      )}
      <p className="muted">Commits from the panel do not run git hooks.</p>
      <PullRequest session={session} gitKey={gitKey} onError={onError} />
    </div>
  );
}

interface RowProps {
  file: ChangedFile;
  sessionId: string;
  open: boolean;
  version: number;
  edit?: FileChange;
  busy: boolean;
  onToggle: () => void;
  onStage: (on: boolean) => void;
  onDiscard: () => void;
  onError: (err: unknown) => void;
}

function FileRow({ file, sessionId, open, version, edit, busy, onToggle, onStage, onDiscard, onError }: RowProps) {
  const [diff, setDiff] = useState<FileDiff | null>(null);
  const [confirming, setConfirming] = useState(false);
  const box = useRef<HTMLInputElement>(null);
  const uncommitted = file.staged || file.unstaged;

  useEffect(() => {
    if (box.current) box.current.indeterminate = file.staged && file.unstaged;
  }, [file.staged, file.unstaged]);
  useEffect(() => {
    if (!open) return;
    EngineService.FileDiff(sessionId, file.path).then(setDiff).catch(onError);
  }, [open, version, sessionId, file.path]);

  const rows = diff ? parseDiff(diff.diff) : [];
  const stats = diffStats(rows);
  return (
    <li className="diff-file">
      <div className="diff-file-row">
        {uncommitted ? (
          <input
            ref={box}
            type="checkbox"
            checked={file.staged && !file.unstaged}
            disabled={busy}
            onChange={() => onStage(!(file.staged && !file.unstaged))}
            aria-label={`Stage ${file.path}`}
            title={file.staged ? (file.unstaged ? "Partly staged; click to stage all of it" : "Staged") : "Not staged"}
          />
        ) : (
          <span className="diff-committed" title="Committed since the session started">
            ✓
          </span>
        )}
        <span className={`diff-status diff-status-${file.status}`} title={file.status}>
          {STATUS_MARK[file.status] ?? "?"}
        </span>
        <button className="link diff-path" onClick={onToggle} aria-expanded={open}>
          <code>{file.path}</code>
        </button>
        {edit && (
          <button className="link muted diff-agent" onClick={() => showTool(edit.lastToolCallId)} title="Show the agent's latest edit">
            agent
          </button>
        )}
      </div>
      {open && (
        <div className="diff-body">
          {!diff && <p className="muted">Loading…</p>}
          {diff && (
            <>
              <div className="row diff-tools">
                <span className="muted">
                  <span className="diff-plus">+{stats.added}</span> <span className="diff-minus">−{stats.deleted}</span>
                </span>
                {uncommitted &&
                  (confirming ? (
                    <>
                      <button className="danger" disabled={busy} onClick={() => (setConfirming(false), onDiscard())}>
                        {file.status === "untracked" ? "Delete the file" : "Discard uncommitted changes"}
                      </button>
                      <button onClick={() => setConfirming(false)}>Keep</button>
                    </>
                  ) : (
                    <button onClick={() => setConfirming(true)} disabled={busy}>
                      Discard…
                    </button>
                  ))}
              </div>
              <pre className="diff-view">
                {rows.map((r, i) => (
                  <div key={i} className={`diff-line diff-${r.kind}`}>
                    <span className="diff-no">{r.oldNo ?? ""}</span>
                    <span className="diff-no">{r.newNo ?? ""}</span>
                    <span className="diff-text">
                      {r.kind === "add" ? "+" : r.kind === "del" ? "-" : r.kind === "ctx" ? " " : ""}
                      {r.text}
                    </span>
                  </div>
                ))}
              </pre>
              {diff.truncated && <p className="muted">The diff is longer than 1 MB and was cut.</p>}
            </>
          )}
        </div>
      )}
    </li>
  );
}

// relative makes an agent's path relative to the project root, as git's are.
function relative(path: string, root: string): string {
  const r = root.endsWith("/") ? root : root + "/";
  if (path.startsWith(r)) return path.slice(r.length);
  return path.replace(/^\.\//, "");
}
