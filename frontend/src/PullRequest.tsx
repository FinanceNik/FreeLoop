import { useEffect, useEffectEvent, useState } from "react";
import { Browser } from "@wailsio/runtime";
import { EngineService, type ChecksInfo, type PullRequestInfo, type Session } from "./api.ts";
import { t, type PlainKey } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";

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

  const failed = useEffectEvent((err: unknown) => onError(err));
  useEffect(() => {
    EngineService.PullRequestInfo(session.id)
      .then((i) => {
        setInfo(i);
        setBase((b) => b || i.base);
        setTitle((cur) => cur || i.title);
      })
      .catch(failed);
  }, [session.id, gitKey]);

  if (!info) return null;
  const opened = info.opened;
  const open = (url: string) => Browser.OpenURL(url).catch(onError);

  const suggest = () => {
    setBusy("suggest");
    EngineService.SuggestPullRequest(session.id)
      .then((text) => {
        setTitle(text.title);
        setBody(text.body);
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
    <section className="pr-form" aria-label={t("pr.title")}>
      <h3>{t("pr.title")}</h3>
      {opened && (
        <p className="muted">
          {tNodes("pr.opened", {
            link: (
              <a
                href={opened.url}
                onClick={(e) => {
                  e.preventDefault();
                  open(opened.url);
                }}
              >
                {opened.title}
              </a>
            ),
          })}
        </p>
      )}
      {(opened || created) && <Checks sessionId={session.id} url={opened?.url ?? created} onMessage={onMessage} onError={onError} />}
      {info.blocked ? (
        <p className="muted">{info.blocked}</p>
      ) : (
        <>
          <p className="muted">
            {tNodes("pr.target", {
              count: (info.commits ?? []).length,
              branch: <code>{info.branch}</code>,
              target: <input className="pr-base" aria-label={t("pr.targetBranch")} value={base} onChange={(e) => setBase(e.target.value)} size={Math.max(base.length, 4)} />,
              remote: info.remote,
            })}
          </p>
          <input aria-label={t("pr.titleLabel")} placeholder={t("pr.titlePlaceholder")} value={title} onChange={(e) => setTitle(e.target.value)} />
          <textarea aria-label={t("pr.description")} rows={6} placeholder={t("pr.descriptionPlaceholder")} value={body} onChange={(e) => setBody(e.target.value)} />
          <div className="row">
            <button
              className="primary"
              disabled={!!busy || !title.trim() || !base.trim()}
              onClick={create}
              title={t("pr.createTitle", { branch: info.branch, remote: info.remote })}
            >
              {busy === "create" ? t("pr.creating") : t("pr.create")}
            </button>
            <button disabled={!!busy} onClick={suggest} title={t("diff.asksModel")}>
              {busy === "suggest" ? t("pr.drafting") : t("pr.draftDescription")}
            </button>
            <label className="row-label">
              <input type="checkbox" checked={draft} onChange={(e) => setDraft(e.target.checked)} /> {t("pr.draft")}
            </label>
          </div>
          {created && (
            <p role="status">
              {tNodes("pr.created", {
                link: (
                  <a
                    href={created}
                    onClick={(e) => {
                      e.preventDefault();
                      open(created);
                    }}
                  >
                    {created}
                  </a>
                ),
              })}
            </p>
          )}
          <p className="muted">{t("pr.note")}</p>
        </>
      )}
    </section>
  );
}

const BUCKET: Record<string, { mark: string; label: PlainKey }> = {
  pass: { mark: "✓", label: "pr.check.passed" },
  fail: { mark: "✕", label: "pr.check.failed" },
  cancel: { mark: "✕", label: "pr.check.cancelled" },
  pending: { mark: "•", label: "pr.check.running" },
  skipping: { mark: "–", label: "pr.check.skipped" },
};

// bucketOf is a check's bucket as gh reports it, when the panel knows it.
const bucketOf = (bucket: string) => (Object.hasOwn(BUCKET, bucket) ? BUCKET[bucket] : undefined);

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
  const loadNow = useEffectEvent(load);
  useEffect(() => {
    loadNow();
  }, [sessionId, url]);
  useEffect(() => {
    if (!checks?.pending) return;
    const timer = setInterval(() => loadNow(), 30_000);
    return () => clearInterval(timer);
  }, [checks?.pending]);
  const send = () => {
    setBusy(true);
    EngineService.CheckFailures(sessionId)
      .then(onMessage)
      .catch(onError)
      .finally(() => setBusy(false));
  };
  return (
    <div className="pr-checks" aria-label={t("pr.checks")}>
      <div className="row">
        <strong>{t("pr.checks")}</strong>
        {checks && (
          <span className="muted">
            {(checks.checks ?? []).length === 0
              ? t("pr.checks.none")
              : [
                  checks.failed ? t("pr.checks.failed", { count: checks.failed }) : "",
                  checks.pending ? t("pr.checks.running", { count: checks.pending }) : "",
                  checks.passed ? t("pr.checks.passed", { count: checks.passed }) : "",
                ]
                  .filter(Boolean)
                  .join(" · ")}
          </span>
        )}
        <button className="link" onClick={load}>
          {t("diff.refresh")}
        </button>
      </div>
      {problem && <p className="muted">{problem}</p>}
      {checks && (checks.checks ?? []).length > 0 && (
        <ul className="pr-check-list">
          {(checks.checks ?? []).map((c, i) => {
            // The panel's word for a bucket it knows, else gh's own, as given.
            const bucket = bucketOf(c.bucket);
            const label = bucket ? t(bucket.label) : undefined;
            return (
              <li key={i} className={`pr-check pr-check-${c.bucket}`}>
                <span className="pr-check-mark" aria-label={label ?? c.state}>
                  {bucket?.mark ?? "?"}
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
                <span className="muted"> {label ?? c.state.toLowerCase()}</span>
              </li>
            );
          })}
        </ul>
      )}
      {checks && checks.failed > 0 && (
        <button disabled={busy} onClick={send} title={t("pr.checks.sendTitle")}>
          {busy ? t("pr.checks.reading") : t("pr.checks.send")}
        </button>
      )}
    </div>
  );
}
