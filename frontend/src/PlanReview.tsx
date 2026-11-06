import { useEffect, useMemo, useState } from "react";
import { EngineService } from "./api.ts";
import { Markdown } from "./Markdown.tsx";
import { approveLabel, planBlocks, planChoices, planFeedback, planVersions, quoteOf, type PlanNote, type PlanVersion } from "./planView.ts";
import { lineDiff, type Item } from "./transcript.ts";

// Reviewing a plan (GUI-043): read it formatted, comment on its parts, approve it with
// the mode to go on in, or send it back with what should change.

type ToolItem = Extract<Item, { kind: "tool" }>;

// REVIEW_PLAN_EVENT asks the session's view to open its Plan tab.
export const REVIEW_PLAN_EVENT = "freeloop-review-plan";

const STATE_LABEL: Record<PlanVersion["state"], string> = {
  waiting: "Waiting for you",
  approved: "Approved",
  "sent back": "Sent back",
  stopped: "Stopped",
  shown: "Shown",
};

// PlanDecision approves or sends back a waiting plan. feedback is what goes back with
// "keep planning": the comments, if any, and the note typed here.
export function PlanDecision({ item, comments = [], onSent, onError }: { item: ToolItem; comments?: PlanNote[]; onSent?: () => void; onError: (err: unknown) => void }) {
  const options = item.permission?.options ?? [];
  const { approve, back } = planChoices(options);
  const [chosen, setChosen] = useState(approve[0]?.optionId ?? "");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const decide = (optionId: string, reason: string) => {
    setBusy(true);
    EngineService.Decide({ requestId: item.permission!.requestId, optionId, reason, scope: "", answers: undefined })
      .then(() => onSent?.())
      .catch(onError)
      .finally(() => setBusy(false));
  };
  const feedback = planFeedback(comments, note);
  const counted = comments.filter((c) => c.note.trim()).length;
  return (
    <div className="plan-decision">
      <div className="plan-decision-row">
        {approve.length > 1 && (
          <select aria-label="Then go on" value={chosen} onChange={(e) => setChosen(e.target.value)}>
            {approve.map((o) => (
              <option key={o.optionId} value={o.optionId}>
                {approveLabel(o)}
              </option>
            ))}
          </select>
        )}
        {approve.length > 0 && (
          <button className="primary" disabled={busy || !chosen} onClick={() => decide(chosen, "")}>
            Approve
          </button>
        )}
      </div>
      <textarea aria-label="What should change" placeholder="What should change? Sent with Keep planning." rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
      <div className="plan-decision-row">
        {back.length > 0 && (
          <button disabled={busy} onClick={() => decide(back[0].optionId, feedback)}>
            Keep planning{counted > 0 ? ` (${counted} ${counted === 1 ? "comment" : "comments"})` : ""}
          </button>
        )}
      </div>
    </div>
  );
}

// PlanCard is a plan in the conversation: formatted, and a way to the full review.
export function PlanCard({ text }: { text: string }) {
  return (
    <div className="plan-card">
      <div className="plan md">
        <Markdown text={text} />
      </div>
      <button className="link plan-review-link" onClick={(e) => e.currentTarget.dispatchEvent(new CustomEvent(REVIEW_PLAN_EVENT, { bubbles: true }))}>
        Review in the panel
      </button>
    </div>
  );
}

// PlanPanel is the panel's Plan tab: the session's plans, the latest first shown, what
// changed since the one before, and comments on a waiting plan's parts.
export function PlanPanel({ items, onError }: { items: readonly Item[]; onError: (err: unknown) => void }) {
  const versions = useMemo(() => planVersions(items), [items]);
  const [at, setAt] = useState<number | null>(null);
  const shown = at === null || at >= versions.length ? versions.length - 1 : at;
  const version = versions[shown];
  const [changes, setChanges] = useState(false);
  const [comments, setComments] = useState<PlanNote[]>([]);
  const [open, setOpen] = useState<number | null>(null);
  // A new plan or another version starts without comments.
  useEffect(() => {
    setComments([]);
    setOpen(null);
  }, [version?.toolCallId]);
  // A new plan arriving shows itself.
  useEffect(() => setAt(null), [versions.length]);

  if (!version) return <p className="muted">No plan in this session yet. In plan mode the agent shows its plan here before it changes anything.</p>;
  const waiting = version.state === "waiting";
  const blocks = planBlocks(version.text);
  const previous = shown > 0 ? versions[shown - 1] : null;
  const noteOf = (i: number) => comments.find((c) => c.block === i)?.note ?? "";
  const setNote = (i: number, note: string) =>
    setComments((list) => [...list.filter((c) => c.block !== i), { block: i, quote: quoteOf(blocks[i]), note }]);
  return (
    <div className="plan-panel">
      <div className="plan-panel-head">
        {versions.length > 1 ? (
          <select aria-label="Version" value={shown} onChange={(e) => setAt(Number(e.target.value))}>
            {versions.map((v, i) => (
              <option key={v.toolCallId} value={i}>
                Version {i + 1} of {versions.length}: {STATE_LABEL[v.state]}
              </option>
            ))}
          </select>
        ) : (
          <span className="muted">{STATE_LABEL[version.state]}</span>
        )}
        {previous && (
          <label className="plan-changes-toggle">
            <input type="checkbox" checked={changes} onChange={(e) => setChanges(e.target.checked)} /> Changes since version {shown}
          </label>
        )}
      </div>
      {changes && previous ? (
        <pre className="plan-diff" aria-label={`Changes since version ${shown}`}>
          {lineDiff(previous.text, version.text, 3).map((l, i) => (
            <div key={i} className={l.sign === "+" ? "add" : l.sign === "-" ? "del" : ""}>
              {l.sign} {l.text}
            </div>
          ))}
        </pre>
      ) : (
        <div className="plan-blocks">
          {blocks.map((b, i) => (
            <div key={i} className={noteOf(i).trim() ? "plan-block plan-block-noted" : "plan-block"}>
              <div className="md">
                <Markdown text={b} />
              </div>
              {waiting && (
                <>
                  {open === i || noteOf(i) ? (
                    <textarea
                      className="plan-comment"
                      aria-label={`Comment on: ${quoteOf(b)}`}
                      placeholder="Your comment on this part"
                      rows={2}
                      autoFocus={open === i}
                      value={noteOf(i)}
                      onChange={(e) => setNote(i, e.target.value)}
                    />
                  ) : (
                    <button className="link plan-comment-add" onClick={() => setOpen(i)} aria-label={`Comment on: ${quoteOf(b)}`}>
                      Comment
                    </button>
                  )}
                </>
              )}
            </div>
          ))}
        </div>
      )}
      {waiting && <PlanDecision key={version.toolCallId} item={version.item} comments={comments} onError={onError} />}
      {version.state === "sent back" && version.item.permission?.decided?.reason && <p className="muted">Sent back with: {version.item.permission.decided.reason}</p>}
    </div>
  );
}
