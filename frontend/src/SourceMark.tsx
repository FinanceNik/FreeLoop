import type { CSSProperties } from "react";
import { sourceColor, type Source } from "./source.ts";

// SourceMark is the chip that says what a session runs on (GUI-034).
export default function SourceMark({ source, detail }: { source: Source; detail?: string }) {
  const name = `Provider: ${source.name}${detail ? ` · ${detail}` : ""}`;
  return (
    <span className="source-mark" role="img" aria-label={name} title={name} style={{ "--source": sourceColor(source.slot) } as CSSProperties}>
      <span aria-hidden="true">{source.letters}</span>
    </span>
  );
}
