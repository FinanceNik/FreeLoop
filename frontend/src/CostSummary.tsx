import { Clipboard } from "@wailsio/runtime";
import { useEffect, useMemo, useRef, useState } from "react";
import { EngineService } from "./api.ts";
import { drawSummary, summaryText, type CostSummary } from "./costsummary.ts";
import { basename } from "./paths.ts";

// CostSummaryPanel shows the summary to share (GUI-029), as text to paste and as a
// picture to save. Nothing leaves the app unless the user pastes or sends it.
export default function CostSummaryPanel({ summary, onClose, onError }: { summary: CostSummary; onClose: () => void; onError: (err: unknown) => void }) {
  const text = useMemo(() => summaryText(summary), [summary]);
  const canvas = useRef<HTMLCanvasElement>(null);
  const [said, setSaid] = useState("");
  useEffect(() => {
    const ctx = canvas.current?.getContext("2d");
    if (ctx) drawSummary(summary, ctx);
  }, [summary]);
  const say = (message: string) => {
    setSaid(message);
    setTimeout(() => setSaid(""), 4000);
  };
  const copy = () =>
    Clipboard.SetText(text)
      .catch(() => navigator.clipboard?.writeText(text))
      .then(() => say("Copied"))
      .catch(onError);
  const save = () => {
    const url = canvas.current?.toDataURL("image/png") ?? "";
    EngineService.SaveUsagePicture(url.slice(url.indexOf(",") + 1))
      .then((path) => path && say(`Saved to ${basename(path)}`))
      .catch(onError);
  };
  return (
    <section className="usage-card cost-summary" aria-label="Cost summary to share">
      <div className="usage-card-head">
        <h3>Summary to share</h3>
        <span className="row">
          {said && <span className="muted" role="status">{said}</span>}
          <button onClick={copy}>Copy as text</button>
          <button onClick={save}>Save picture…</button>
          <button onClick={onClose} aria-label="Close the summary">
            ×
          </button>
        </span>
      </div>
      <p className="muted">It shows agents and models, not sessions or projects. Nothing is sent anywhere unless you paste or send it.</p>
      <canvas ref={canvas} className="cost-summary-picture" role="img" aria-label={text} />
    </section>
  );
}
