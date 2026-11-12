import { useEffect, useEffectEvent, useRef, useState } from "react";
import { chord, primaryKey } from "./platform.ts";
import { ask } from "./ask.ts";
import { EngineService, type ChangedFile, type Changes, type FileDiff, type Session } from "./api.ts";
import { diffStats, parseDiff, type DiffRow } from "./diff.ts";
import { anchor, locate, reviewMessage, type ReviewComment } from "./review.ts";
import type { FileChange } from "./transcript.ts";
import { showTool } from "./SessionPanel.tsx";
import { usePaletteActions } from "./Palette.tsx";
import PullRequest from "./PullRequest.tsx";
import AgentReview from "./AgentReview.tsx";
import type { PaletteItem } from "./palette.ts";
import { relative } from "./paths.ts";
import { t, type PlainKey } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";

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
  // Puts the review into the composer (GUI-019).
  onReview: (text: string) => void;
  onError: (err: unknown) => void;
}

// Review comments wait here per session until sent, so they outlive
// switching tabs, panes or sessions (not a restart).
const pendingReviews = new Map<string, ReviewComment[]>();
let nextComment = 0;

const STATUS_MARK: Record<string, string> = { added: "A", modified: "M", deleted: "D", untracked: "U" };
const STATUS_TITLE: Record<string, PlainKey> = {
  added: "diff.status.added",
  modified: "diff.status.modified",
  deleted: "diff.status.deleted",
  untracked: "diff.status.untracked",
};

export default function DiffPanel({ session, gitKey, agentEdits, onReview, onError }: Props) {
  const [changes, setChanges] = useState<Changes | null>(null);
  const [comments, setCommentsState] = useState<ReviewComment[]>(() => pendingReviews.get(session.id) ?? []);
  useEffect(() => setCommentsState(pendingReviews.get(session.id) ?? []), [session.id]);
  const setComments = (next: ReviewComment[]) => {
    if (next.length) pendingReviews.set(session.id, next);
    else pendingReviews.delete(session.id);
    setCommentsState(next);
  };
  const sendReview = () => {
    onReview(reviewMessage(comments));
    setComments([]);
  };
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
  const loadNow = useEffectEvent(load);
  useEffect(() => {
    loadNow();
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
  const ready = !!changes?.repo && !changes.blocked;
  const live = ready ? (changes.files ?? []) : [];
  const liveStaged = live.filter((f) => f.staged);
  const liveUncommitted = live.filter((f) => f.staged || f.unstaged);
  // The palette's group, by its name in palette.ts; not text.
  const item = (id: string, title: string, run: () => void, extra: Partial<PaletteItem> = {}): PaletteItem => ({
    id: `diff:${session.id}:${id}`,
    title,
    group: "Changes",
    run,
    ...extra,
  });
  usePaletteActions(
    ready
      ? [
          item("refresh", t("diff.palette.refresh"), load),
          ...(liveUncommitted.some((f) => f.unstaged) ? [item("stage", t("diff.palette.stageAll"), () => act(EngineService.StageFiles(session.id, liveUncommitted.map((f) => f.path), true)))] : []),
          ...(liveStaged.length ? [item("unstage", t("diff.palette.unstageAll"), () => act(EngineService.StageFiles(session.id, liveStaged.map((f) => f.path), false)))] : []),
          ...(liveStaged.length
            ? [item("commit", t("diff.palette.commit", { count: liveStaged.length }), () => messageBox.current?.focus(), { detail: t("diff.palette.commitDetail") })]
            : []),
          ...(liveUncommitted.length ? [item("suggest", t("diff.palette.suggest"), () => suggest(), { detail: t("diff.palette.usesTokens") })] : []),
          ...(session.worktree ? [item("merge", t("diff.palette.merge"), () => setMerging(true), { keywords: "worktree branch" })] : []),
          ...(comments.length ? [item("review", t("diff.palette.review", { count: comments.length }), sendReview, { keywords: "comments feedback" })] : []),
          ...live.flatMap((f) => [
            item(`open:${f.path}`, t("diff.palette.show", { path: f.path }), () => setOpen(f.path), { keywords: "file" }),
            ...(f.unstaged ? [item(`stage:${f.path}`, t("diff.stageFile", { path: f.path }), () => act(EngineService.StageFiles(session.id, [f.path], true)))] : []),
            ...(f.staged ? [item(`unstage:${f.path}`, t("diff.palette.unstage", { path: f.path }), () => act(EngineService.StageFiles(session.id, [f.path], false)))] : []),
            ...(f.staged || f.unstaged
              ? [
                  item(`discard:${f.path}`, t("diff.palette.discard", { path: f.path }), async () => {
                    if ((await ask(t("diff.discardFile.question", { path: f.path })))) act(EngineService.DiscardFile(session.id, f.path));
                  }),
                ]
              : []),
          ]),
        ]
      : [],
  );

  if (!changes) return <p className="muted">{t("diff.loading")}</p>;
  if (!changes.repo) return <p className="muted">{t("diff.notRepo")}</p>;
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
          {changes.base
            ? tNodes("diff.head.since", { count: files.length, base: <code title={changes.base}>{changes.base.slice(0, 8)}</code> })
            : t("diff.head.sinceFirst", { count: files.length })}
        </span>
        <button className="link" onClick={load} disabled={busy}>
          {t("diff.refresh")}
        </button>
      </div>
      {session.worktree && (
        <div className="worktree-bar">
          <span className="muted">{tNodes("diff.worktree.on", { branch: <code>{session.branch}</code> })}</span>
          {merging ? (
            <span className="row">
              <button
                className="primary"
                disabled={busy}
                onClick={() =>
                  act(
                    EngineService.MergeRisks(session.id).then(async (risky) => {
                      const list = risky ?? [];
                      if (list.length > 0 && !(await ask(t("diff.merge.risky", { files: list.join("\n") })))) return;
                      const r = await EngineService.MergeWorktree(session.id, list.length > 0);
                      setMerged(r.message);
                      setMerging(false);
                    }),
                  )
                }
              >
                {t("diff.merge.confirm")}
              </button>
              <button onClick={() => setMerging(false)}>{t("common.cancel")}</button>
            </span>
          ) : (
            <button onClick={() => setMerging(true)} title={t("diff.merge.title")}>
              {t("diff.merge")}
            </button>
          )}
        </div>
      )}
      {merged && <p className="ok">{merged}</p>}
      {comments.length > 0 && (
        <div className="review-bar" role="group" aria-label={t("diff.review.label")}>
          <span>{t("diff.review.count", { count: comments.length })}</span>
          <span className="row">
            <button className="primary" onClick={sendReview} title={t("diff.review.sendTitle")}>
              {t("diff.review.send")}
            </button>
            <button
              onClick={async () => {
                if ((await ask(t("diff.review.discardQuestion", { count: comments.length })))) setComments([]);
              }}
            >
              {t("diff.review.discard")}
            </button>
          </span>
        </div>
      )}
      {files.length > 0 && comments.length === 0 && <p className="muted review-hint">{t("diff.review.hint")}</p>}
      {files.length === 0 && <p className="muted">{t("diff.noChanges")}</p>}
      {uncommitted.length > 0 && (
        <div className="row">
          <button
            disabled={busy || uncommitted.every((f) => f.staged && !f.unstaged)}
            onClick={() => act(EngineService.StageFiles(session.id, uncommitted.map((f) => f.path), true))}
          >
            {t("diff.stageAll")}
          </button>
          <button disabled={busy || staged.length === 0} onClick={() => act(EngineService.StageFiles(session.id, staged.map((f) => f.path), false))}>
            {t("diff.unstageAll")}
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
            comments={comments.filter((c) => c.path === f.path)}
            onComment={(c) => {
              // The same words on the same line once, however often the
              // key fires.
              if (comments.some((x) => x.path === f.path && x.side === c.side && x.line === c.line && x.body === c.body)) return;
              setComments([...comments, { ...c, id: `c${++nextComment}`, path: f.path }]);
            }}
            onUncomment={(id) => setComments(comments.filter((c) => c.id !== id))}
            onError={onError}
          />
        ))}
      </ul>

      <h3>{t("diff.commit.title")}</h3>
      <textarea
        className="commit-message"
        ref={messageBox}
        rows={4}
        placeholder={t("diff.commit.message")}
        aria-label={t("diff.commit.message")}
        value={message}
        onChange={(e) => setMessage(e.target.value)}
      />
      <div className="row">
        <button className="primary" disabled={busy || staged.length === 0 || !message.trim()} onClick={commit}>
          {t("diff.commit.button", { count: staged.length })}
        </button>
        <button disabled={suggesting || uncommitted.length === 0} onClick={suggest} title={t("diff.asksModel")}>
          {suggesting ? t("diff.commit.suggesting") : t("diff.commit.suggest")}
        </button>
      </div>
      {committed && <p className="muted">{tNodes("diff.committed", { hash: <code>{committed.slice(0, 8)}</code> })}</p>}
      <p className="muted">{t("diff.commit.noHooks")}</p>
      {files.length > 0 && !session.parentId && <AgentReview session={session} gitKey={gitKey} onMessage={onReview} onError={onError} />}
      <PullRequest session={session} gitKey={gitKey} onMessage={onReview} onError={onError} />
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
  comments: ReviewComment[];
  onComment: (c: Omit<ReviewComment, "id" | "path">) => void;
  onUncomment: (id: string) => void;
  onError: (err: unknown) => void;
}

function FileRow({ file, sessionId, open, version, edit, busy, onToggle, onStage, onDiscard, comments, onComment, onUncomment, onError }: RowProps) {
  const [diff, setDiff] = useState<FileDiff | null>(null);
  const [confirming, setConfirming] = useState(false);
  // The row a comment is being written on, and its text.
  const [writing, setWriting] = useState<number | null>(null);
  const [body, setBody] = useState("");
  const box = useRef<HTMLInputElement>(null);
  const uncommitted = file.staged || file.unstaged;
  const statusTitle = Object.hasOwn(STATUS_TITLE, file.status) ? STATUS_TITLE[file.status] : undefined;

  useEffect(() => {
    if (box.current) box.current.indeterminate = file.staged && file.unstaged;
  }, [file.staged, file.unstaged]);
  const failed = useEffectEvent((err: unknown) => onError(err));
  useEffect(() => {
    if (!open) return;
    EngineService.FileDiff(sessionId, file.path).then(setDiff).catch(failed);
  }, [open, version, sessionId, file.path]);

  const rows = diff ? parseDiff(diff.diff) : [];
  const stats = diffStats(rows);
  // Each comment at its line in the current diff; one whose line changed
  // since is listed above the diff.
  const placed = new Map<number, ReviewComment[]>();
  const lost: ReviewComment[] = [];
  for (const c of comments) {
    const i = locate(rows, c);
    if (i < 0) lost.push(c);
    else placed.set(i, [...(placed.get(i) ?? []), c]);
  }
  const add = () => {
    const a = writing !== null ? anchor(rows, writing) : null;
    if (a && body.trim()) onComment({ ...a, body: body.trim() });
    setWriting(null);
    setBody("");
  };
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
            aria-label={t("diff.stageFile", { path: file.path })}
            title={file.staged ? (file.unstaged ? t("diff.file.partlyStaged") : t("diff.file.staged")) : t("diff.file.notStaged")}
          />
        ) : (
          <span className="diff-committed" title={t("diff.file.committed")}>
            ✓
          </span>
        )}
        <span className={`diff-status diff-status-${file.status}`} title={statusTitle ? t(statusTitle) : file.status}>
          {STATUS_MARK[file.status] ?? "?"}
        </span>
        <button className="link diff-path" onClick={onToggle} aria-expanded={open}>
          <code>{file.path}</code>
        </button>
        {comments.length > 0 && (
          <span className="badge review-count" title={t("diff.file.comments")}>
            {comments.length}
          </span>
        )}
        {edit && (
          <button className="link muted diff-agent" onClick={() => showTool(edit.lastToolCallId)} title={t("diff.file.agentEditTitle")}>
            {t("diff.file.agentEdit")}
          </button>
        )}
      </div>
      {open && (
        <div className="diff-body">
          {!diff && <p className="muted">{t("diff.loading")}</p>}
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
                        {file.status === "untracked" ? t("diff.file.delete") : t("diff.file.discardUncommitted")}
                      </button>
                      <button onClick={() => setConfirming(false)}>{t("diff.file.keep")}</button>
                    </>
                  ) : (
                    <button onClick={() => setConfirming(true)} disabled={busy}>
                      {t("diff.file.discard")}
                    </button>
                  ))}
              </div>
              {lost.map((c) => (
                <CommentView key={c.id} c={c} changed onRemove={() => onUncomment(c.id)} />
              ))}
              <pre className="diff-view">
                {rows.map((r, i) => (
                  <div key={i}>
                    <div className={`diff-line diff-${r.kind}`}>
                      <DiffNo row={r} side="old" onComment={commentable(r) ? () => (setWriting(i), setBody("")) : undefined} />
                      <DiffNo row={r} side="new" onComment={commentable(r) ? () => (setWriting(i), setBody("")) : undefined} />
                      <span className="diff-text">
                        {r.kind === "add" ? "+" : r.kind === "del" ? "-" : r.kind === "ctx" ? " " : ""}
                        {r.text}
                      </span>
                    </div>
                    {(placed.get(i) ?? []).map((c) => (
                      <CommentView key={c.id} c={c} onRemove={() => onUncomment(c.id)} />
                    ))}
                    {writing === i && (
                      <div className="review-editor">
                        <textarea
                          autoFocus
                          rows={3}
                          aria-label={t("diff.comment.editor", { line: lineNo(r.newNo ?? r.oldNo), path: file.path })}
                          placeholder={t("diff.comment.placeholder")}
                          value={body}
                          onChange={(e) => setBody(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === "Enter" && primaryKey(e)) {
                              e.preventDefault();
                              if (!e.repeat) add();
                            } else if (e.key === "Escape") {
                              e.preventDefault();
                              setWriting(null);
                            }
                          }}
                        />
                        <div className="row">
                          <button className="primary" disabled={!body.trim()} onClick={add} title={chord("Enter")}>
                            {t("diff.comment.add")}
                          </button>
                          <button onClick={() => setWriting(null)}>{t("common.cancel")}</button>
                        </div>
                      </div>
                    )}
                  </div>
                ))}
              </pre>
              {diff.truncated && <p className="muted">{t("diff.truncated")}</p>}
            </>
          )}
        </div>
      )}
    </li>
  );
}

const commentable = (r: DiffRow) => r.kind === "add" || r.kind === "del" || r.kind === "ctx";

// lineNo is a line number for a sentence: as a line number is written, without the
// thousands separator a count gets.
const lineNo = (n: number | undefined) => String(n ?? "");

// DiffNo is a line number; on a line of code it is also the button that
// starts a comment there.
function DiffNo({ row, side, onComment }: { row: DiffRow; side: "old" | "new"; onComment?: () => void }) {
  const n = side === "old" ? row.oldNo : row.newNo;
  if (!onComment || n === undefined) return <span className="diff-no">{n ?? ""}</span>;
  return (
    <button
      className="diff-no diff-no-button"
      onClick={onComment}
      aria-label={t(side === "old" ? "diff.comment.onOldLine" : "diff.comment.onLine", { line: lineNo(n) })}
      title={t("diff.comment.onThisLine")}
    >
      {n}
    </button>
  );
}

// A comment's head, by whether its line was removed and whether that line has changed
// since the comment was written.
const COMMENT_HEAD = {
  new: { same: "diff.comment.line", changed: "diff.comment.lineChanged" },
  old: { same: "diff.comment.lineRemoved", changed: "diff.comment.lineRemovedChanged" },
} as const;

function CommentView({ c, changed, onRemove }: { c: ReviewComment; changed?: boolean; onRemove: () => void }) {
  return (
    <div className="review-comment">
      <div className="review-comment-head">
        <span className="muted">{t(COMMENT_HEAD[c.side][changed ? "changed" : "same"], { line: lineNo(c.line) })}</span>
        <button className="icon-button" aria-label={t("diff.comment.remove")} title={t("common.remove")} onClick={onRemove}>
          ×
        </button>
      </div>
      <div className="review-comment-body">{c.body}</div>
    </div>
  );
}
