import { useEffect, useState } from "react";
import { EngineService, type GitHubIssue, type IssueBrief } from "./api.ts";

// IssuePicker lists a trusted project's open GitHub issues for the
// new-session form (SES-017); choosing one hands back its brief, which
// becomes the session's title, tag and first message, to read and edit
// before sending.

export default function IssuePicker({ root, onPick, onClose }: { root: string; onPick: (b: IssueBrief) => void; onClose: () => void }) {
  const [search, setSearch] = useState("");
  const [issues, setIssues] = useState<GitHubIssue[] | null>(null);
  const [problem, setProblem] = useState("");
  const [loading, setLoading] = useState<number | null>(null);
  useEffect(() => {
    let live = true;
    const t = setTimeout(() => {
      EngineService.Issues(root, search)
        .then((list) => {
          if (!live) return;
          setIssues(list ?? []);
          setProblem("");
        })
        .catch((err) => live && setProblem(err instanceof Error ? err.message : String(err)));
    }, 300);
    return () => {
      live = false;
      clearTimeout(t);
    };
  }, [root, search]);
  const pick = (n: number) => {
    setLoading(n);
    EngineService.IssueBrief(root, n)
      .then(onPick)
      .catch((err) => setProblem(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(null));
  };
  return (
    <div className="issue-picker">
      <div className="row">
        <input autoFocus type="search" aria-label="Search issues" placeholder="Search open issues, such as label:bug" value={search} onChange={(e) => setSearch(e.target.value)} />
        <button type="button" onClick={onClose}>
          Cancel
        </button>
      </div>
      {problem && <p className="muted">{problem}</p>}
      {!problem && issues === null && <p className="muted">Asking GitHub…</p>}
      {issues && issues.length === 0 && !problem && <p className="muted">No open issues match.</p>}
      {issues && issues.length > 0 && (
        <ul className="issue-list">
          {issues.map((is) => (
            <li key={is.number}>
              <button type="button" disabled={loading !== null} onClick={() => pick(is.number)}>
                <span className="muted">#{is.number}</span> {is.title}
                {(is.labels ?? []).map((l) => (
                  <span key={l} className="tag">
                    {l}
                  </span>
                ))}
                {loading === is.number && <span className="muted"> reading…</span>}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
