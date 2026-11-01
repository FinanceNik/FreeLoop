import { useEffect, useState } from "react";
import { EngineService, type Profile, type ReviewInfo, type Session } from "./api.ts";
import { openSessionEvent } from "./ToolView.tsx";

// AgentReview is the Diff tab's review by a second agent (HRN-018): another
// profile reads the session's changes in a read-only session of its own,
// and its findings go to this session's message box only when asked.

const WORKING = new Set(["running", "waiting_permission", "waiting_input", "queued"]);

export default function AgentReview({ session, gitKey, onMessage, onError }: { session: Session; gitKey: string; onMessage: (text: string) => void; onError: (err: unknown) => void }) {
  const [profiles, setProfiles] = useState<Profile[]>([]);
  const [profileId, setProfileId] = useState("");
  const [focus, setFocus] = useState("");
  const [reviews, setReviews] = useState<ReviewInfo[]>([]);
  const [busy, setBusy] = useState(false);
  const load = () =>
    EngineService.Reviews(session.id)
      .then((r) => setReviews(r ?? []))
      .catch(onError);
  useEffect(() => {
    load();
    Promise.resolve(EngineService.ListProfiles())
      .then((list) => {
        const all = list ?? [];
        setProfiles(all);
        // Another profile than the session's is the point of a second opinion.
        setProfileId((id) => id || all.find((p) => p.id !== session.profileId)?.id || all[0]?.id || "");
      })
      .catch(onError);
  }, [session.id, gitKey]);
  // A review that was just asked for has not started yet, so one without a
  // reply counts as under way too.
  const waiting = reviews.some((r) => WORKING.has(r.session.status) || !r.reply);
  useEffect(() => {
    if (!waiting) return;
    const t = setInterval(load, 3000);
    return () => clearInterval(t);
  }, [waiting, session.id]);

  const ask = () => {
    setBusy(true);
    EngineService.RequestReview({ sessionId: session.id, profileId, label: profiles.find((p) => p.id === profileId)?.name ?? profileId, focus })
      .then(() => {
        setFocus("");
        return load();
      })
      .catch(onError)
      .finally(() => setBusy(false));
  };
  const useFindings = (id: string) => EngineService.ReviewFindings(id).then(onMessage).catch(onError);

  return (
    <section className="agent-review" aria-label="Review by a second agent">
      <h3>Second opinion</h3>
      <p className="muted">Another profile reads these changes in a read-only session of its own.</p>
      <div className="row wrap">
        <select aria-label="Reviewer" value={profileId} onChange={(e) => setProfileId(e.target.value)}>
          {profiles.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
        <input aria-label="What to look at" placeholder="Anything to look at? (optional)" value={focus} onChange={(e) => setFocus(e.target.value)} />
        <button disabled={busy || !profileId} onClick={ask}>
          {busy ? "Starting…" : "Ask for a review"}
        </button>
      </div>
      {reviews.length > 0 && (
        <ul className="agent-review-list">
          {reviews.map((r) => (
            <li key={r.session.id}>
              <div className="row">
                <strong>{r.session.title.replace(/^Review: /, "")}</strong>
                <span className={`badge badge-${r.session.status}`}>{WORKING.has(r.session.status) || !r.reply ? "Reviewing" : "Done"}</span>
              </div>
              {r.reply && <p className="agent-review-reply">{r.reply}</p>}
              <div className="row">
                <button className="link" onClick={() => openSessionEvent(r.session.id)}>
                  Open
                </button>
                {r.reply && !WORKING.has(r.session.status) && (
                  <button onClick={() => useFindings(r.session.id)} title="Puts the findings in the message box, to edit and send">
                    Use these findings
                  </button>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
