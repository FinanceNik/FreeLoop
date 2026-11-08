import type { CSSProperties } from "react";
import { t } from "./i18n.ts";
import { sourceColor, type Source } from "./source.ts";

// SourceMark is the chip that says what a session runs on (GUI-034).
export default function SourceMark({ source, detail }: { source: Source; detail?: string }) {
  const name = t("marks.provider", { name: source.name }) + (detail ? ` · ${detail}` : "");
  return (
    <span className="source-mark" role="img" aria-label={name} title={name} style={{ "--source": sourceColor(source.slot) } as CSSProperties}>
      <span aria-hidden="true">{source.letters}</span>
    </span>
  );
}
