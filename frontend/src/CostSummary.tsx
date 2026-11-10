import { Clipboard } from "@wailsio/runtime";
import { useEffect, useMemo, useRef, useState } from "react";
import { EngineService } from "./api.ts";
import { drawSummary, summaryText, type CostSummary } from "./costsummary.ts";
import { locale, t } from "./i18n.ts";
import { basename } from "./paths.ts";

// CostSummaryPanel shows the summary to share (GUI-029), as text to paste and as a
// picture to save. Nothing leaves the app unless the user pastes or sends it. Its
// headings are in the language in force, so a change of language draws it again.
export default function CostSummaryPanel({ summary, onClose, onError }: { summary: CostSummary; onClose: () => void; onError: (err: unknown) => void }) {
  const text = useMemo(() => summaryText(summary), [summary, locale()]);
  const canvas = useRef<HTMLCanvasElement>(null);
  const [said, setSaid] = useState("");
  useEffect(() => {
    const ctx = canvas.current?.getContext("2d");
    if (ctx) drawSummary(summary, ctx);
  }, [summary, locale()]);
  const say = (message: string) => {
    setSaid(message);
    setTimeout(() => setSaid(""), 4000);
  };
  const copy = () =>
    Clipboard.SetText(text)
      .catch(() => navigator.clipboard?.writeText(text))
      .then(() => say(t("costSummary.copied")))
      .catch(onError);
  const save = () => {
    const url = canvas.current?.toDataURL("image/png") ?? "";
    EngineService.SaveUsagePicture(url.slice(url.indexOf(",") + 1))
      .then((path) => path && say(t("costSummary.savedTo", { file: basename(path) })))
      .catch(onError);
  };
  return (
    <section className="usage-card cost-summary" aria-label={t("costSummary.label")}>
      <div className="usage-card-head">
        <h3>{t("costSummary.title")}</h3>
        <span className="row">
          {said && <span className="muted" role="status">{said}</span>}
          <button onClick={copy}>{t("costSummary.copy")}</button>
          <button onClick={save}>{t("costSummary.save")}</button>
          <button onClick={onClose} aria-label={t("costSummary.close")}>
            ×
          </button>
        </span>
      </div>
      <p className="muted">{t("costSummary.note")}</p>
      <canvas ref={canvas} className="cost-summary-picture" role="img" aria-label={text} />
    </section>
  );
}
