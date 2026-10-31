import { useEffect, useState } from "react";
import { Browser } from "@wailsio/runtime";
import { EngineService, type ChecksInfo, type PullRequestInfo, type Session } from "./api.ts";

// PullRequest is the diff panel's pull request section (SES-011): the
// branch and its target, a title and description the model can draft and
// the user edits, and Create, which pushes the branch and opens the pull
// request with the GitHub CLI.

interface Props {
  session: Session;
  // Changes with the git state, so the commits and the block refresh.
  gitKey: string;
  // Puts a message about failing checks into the composer (SES-018).
  onMessage: (text: string) => void;
  onError: (err: unknown) => void;
}

export default function PullRequest({ session, gitKey, onMessage, onError }: Props) {
  const [info, setInfo] = useState<PullRequestInfo | null>(null);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [base, setBase] = useState("");
  const [draft, setDraft] = useState(false);
  const [busy, setBusy] = useState("");
  const [created, setCreated] = useState("");

  useEffect(() => {
    EngineService.PullRequestInfo(session.id)
      .then((i) => {
        setInfo(i);
        setBase((b) => b || i.base);
        setTitle((t) => t || i.title);
      })
      .catch(onError);
  }, [session.id, gitKey]);

  if (!info) return null;
  const opened = info.opened;
  const open = (url: string) => Browser.OpenURL(url).catch(onError);

  const suggest = () => {
    setBusy("suggest");
    EngineService.SuggestPullRequest(session.id)
      .then((t) => {
        setTitle(t.title);
        setBody(t.body);
      })
      .catch(onError)
      .finally(() => setBusy(""));
  };
  const create = () => {
    setBusy("create");
    EngineService.CreatePullRequest({ sessionId: session.id, title, body, base: base === info.base ? "" : base, draft })
      .then((pr) => {
        setCreated(pr.url);
        setBody("");
      })
      .catch(onError)
      .finally(() => setBusy(""));
  };

  return (
    <section className="pr-form" aria-label="Pull request">
      <h3>Pull request</h3>
      {opened && (
        <p className="muted">
          Opened from this session:{" "}
          <a
            href={opened.url}
            onClick={(e) => {
              e.preventDefault();
              open(opened.url);
            }}
          >
            {opened.title}
          </a>
        </p>
      )}
      {(opened || created) && <Checks sessionId={session.id} url={opened?.url ?? created} onMessage={onMessage} onError={onError} />}
      {info.blocked ? (
        <p className="muted">{info.blocked}</p>
      ) : (
        <>
          <p className="muted">
            <code>{info.branch}</code> into{" "}
            <input className="pr-base" aria-label="Target branch" value={base} onChange={(e) => setBase(e.target.value)} size={Math.max(base.length, 4)} />{" "}
            on {info.remote}: {(info.commits ?? []).length} {(info.commits ?? []).length === 1 ? "commit" : "commits"}
          </p>
          <input aria-label="Pull request title" placeholder="Title" value={title} onChange={(e) => setTitle(e.target.value)} />
          <textarea aria-label="Pull request description" rows={6} placeholder="Description (Markdown)" value={body} onChange={(e) => setBody(e.target.value)} />
          <div className="row">
            <button className="primary" disabled={!!busy || !title.trim() || !base.trim()} onClick={create} title={`Pushes ${info.branch} to ${info.remote} and runs gh pr create`}>
              {busy === "create" ? "Creating…" : "Create pull request"}
            </button>
            <button disabled={!!busy} onClick={suggest} title="Asks the session's model; uses tokens">
              {busy === "suggest" ? "Drafting…" : "Draft description"}
            </button>
            <label className="row-label">
              <input type="checkbox" checked={draft} onChange={(e) => setDraft(e.target.checked)} /> Draft
            </label>
          </div>
          {created && (
            <p role="status">
              Created{" "}
              <a
                href={created}
                onClick={(e) => {
                  e.preventDefault();
                  open(created);
                }}
              >
                {created}
              </a>
            </p>
          )}
          <p className="muted">Pushes without git hooks and opens the pull request with the GitHub CLI (gh), signed in on its own.</p>
        </>
      )}
    </section>
  );
}

const BUCKET: Record<string, { mark: string; label: string }> = {
  pass: { mark: "✓", label: "passed" },
  fail: { mark: "✕", label: "failed" },
  cancel: { mark: "✕", label: "cancelled" },
  pending: { mark: "•", label: "running" },
  skipping: { mark: "–", label: "skipped" },
};

// Checks lists the pull request's CI checks (SES-018), refreshing every
// 30 seconds while any runs, and turns failures into a message.
function Checks({ sessionId, url, onMessage, onError }: { sessionId: string; url: string; onMessage: (text: string) => void; onError: (err: unknown) => void }) {
  const [checks, setChecks] = useState<ChecksInfo | null>(null);
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  const load = () =>
    EngineService.PullRequestChecks(sessionId)
      .then((c) => {
        setChecks({ ...c, checks: c.checks ?? [] });
        setProblem("");
      })
      .catch((err) => setProblem(err instanceof Error ? err.message : String(err)));
  useEffect(() => {
    load();
  }, [sessionId, url]);
  useEffect(() => {
    if (!checks?.pending) return;
    const t = setInterval(load, 30_000);
    return () => clearInterval(t);
  }, [checks?.pending]);
  const send = () => {
    setBusy(true);
    EngineService.CheckFailures(sessionId)
      .then(onMessage)
      .catch(onError)
      .finally(() => setBusy(false));
  };
  return (
    <div className="pr-checks" aria-label="Checks">
      <div className="row">
        <strong>Checks</strong>
        {checks && (
          <span className="muted">
            {(checks.checks ?? []).length === 0
              ? "none reported yet"
              : [checks.failed && `${checks.failed} failed`, checks.pending && `${checks.pending} running`, checks.passed && `${checks.passed} passed`].filter(Boolean).join(" · ")}
          </span>
        )}
        <button className="link" onClick={load}>
          Refresh
        </button>
      </div>
      {problem && <p className="muted">{problem}</p>}
      {checks && (checks.checks ?? []).length > 0 && (
        <ul className="pr-check-list">
          {(checks.checks ?? []).map((c, i) => (
            <li key={i} className={`pr-check pr-check-${c.bucket}`}>
              <span className="pr-check-mark" aria-label={BUCKET[c.bucket]?.label ?? c.state}>
                {BUCKET[c.bucket]?.mark ?? "?"}
              </span>
              {c.link.startsWith("https://") ? (
                <a
                  href={c.link}
                  onClick={(e) => {
                    e.preventDefault();
                    Browser.OpenURL(c.link).catch(onError);
                  }}
                >
                  {c.workflow && c.workflow !== c.name ? `${c.workflow} / ${c.name}` : c.name}
                </a>
              ) : (
                <span>{c.name}</span>
              )}
              <span className="muted"> {BUCKET[c.bucket]?.label ?? c.state.toLowerCase()}</span>
            </li>
          ))}
        </ul>
      )}
      {checks && checks.failed > 0 && (
        <button disabled={busy} onClick={send} title="Puts the failing checks and the end of their logs in the message box">
          {busy ? "Reading the logs…" : "Send failures to the agent"}
        </button>
      )}
    </div>
  );
}
