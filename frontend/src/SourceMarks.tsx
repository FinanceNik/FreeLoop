import type { CSSProperties } from "react";
import { t } from "./i18n.ts";
import { assignSlots, defaultLetters, SLOTS, sourceColor, validLetters, type Named, type SourceChoices } from "./source.ts";
import SourceMark from "./SourceMark.tsx";

// SourceMarks chooses each provider's and agent's colour and letters (GUI-034), every one
// the same way.

type Choice = { slot: number; letters: string };

export default function SourceMarks({ value, providers, harnesses, onChange }: { value: Record<string, Choice> | null | undefined; providers: readonly Named[]; harnesses: readonly Named[]; onChange: (next: Record<string, Choice>) => void }) {
  const list = [...providers, ...harnesses.filter((h) => h.id !== "builtin")];
  const slots = assignSlots(list.map((x) => x.id), value as SourceChoices);
  const set = (id: string, change: Partial<Choice>) => {
    const was = value?.[id] ?? { slot: 0, letters: "" };
    const next = { ...was, ...change };
    const all = { ...(value ?? {}) };
    if (!next.slot && !next.letters) delete all[id];
    else all[id] = next;
    onChange(all);
  };
  return (
    <ul className="source-marks">
      {list.map((x) => {
        const letters = value?.[x.id]?.letters ?? "";
        const bad = letters !== "" && !validLetters(letters);
        return (
          <li key={x.id}>
            <SourceMark source={{ id: x.id, name: x.name, letters: letters && !bad ? letters : defaultLetters(x.name), slot: slots.get(x.id) ?? 0 }} />
            <span className="source-marks-name">{x.name}</span>
            <span className="source-marks-slots" role="group" aria-label={t("marks.colourFor", { name: x.name })}>
              {Array.from({ length: SLOTS }, (_, i) => i + 1).map((slot) => (
                <button
                  key={slot}
                  type="button"
                  className="source-marks-slot"
                  aria-label={t("marks.colour", { slot })}
                  aria-pressed={slots.get(x.id) === slot}
                  style={{ "--source": sourceColor(slot) } as CSSProperties}
                  onClick={() => set(x.id, { slot })}
                />
              ))}
            </span>
            <input
              aria-label={t("marks.lettersFor", { name: x.name })}
              aria-invalid={bad || undefined}
              maxLength={2}
              placeholder={defaultLetters(x.name)}
              value={letters}
              onChange={(e) => set(x.id, { letters: e.target.value })}
            />
            {value?.[x.id] && (
              <button type="button" className="link" aria-label={t("marks.reset", { name: x.name })} onClick={() => set(x.id, { slot: 0, letters: "" })}>
                {t("common.reset")}
              </button>
            )}
          </li>
        );
      })}
    </ul>
  );
}
