import { useEffect, useState } from "react";
import { Browser } from "@wailsio/runtime";
import { EngineService, type PullRequestInfo, type Session } from "./api.ts";

// PullRequest is the diff panel's pull request section (SES-011): the
// branch and its target, a title and description the model can draft and
// the user edits, and Create, which pushes the branch and opens the pull
// request with the GitHub CLI.

interface Props {
  session: Session;
  // Changes with the git state, so the commits and the block refresh.
  gitKey: string;
  onError: (err: unknown) => void;
}

export default function PullRequest({ session, gitKey, onError }: Props) {
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
