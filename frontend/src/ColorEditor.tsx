import { useMemo, useState } from "react";
import { EngineService } from "./api.ts";
import { COLOR_GROUPS, hardToRead, isColor, pickerValue, tokenDefaults, type ColorMap, type ColorMode, type ColorSet } from "./colors.ts";

// The colors of Settings → Appearance (GUI-022). It edits what colors.json
// holds: a change shows at once and reaches the file a moment later, and an
// edit of the file comes back here.

interface Props {
  colors: ColorSet;
  onChange: (c: ColorSet) => void;
  onError: (err: unknown) => void;
}

function shownMode(): ColorMode {
  const theme = document.documentElement.dataset.theme;
  if (theme === "light" || theme === "dark") return theme;
  return window.matchMedia?.("(prefers-color-scheme: dark)").matches ? "dark" : "light";
}

export default function ColorEditor({ colors, onChange, onError }: Props) {
  const [mode, setMode] = useState<ColorMode>(shownMode);
  const defaults = useMemo(() => tokenDefaults(mode), [mode]);
  const set: ColorMap = colors[mode] ?? {};
  const other = shownMode() !== mode;

  const put = (next: ColorMap) => onChange({ ...colors, [mode]: next });
  const setToken = (id: string, value: string) => put({ ...set, [id]: value });
  const reset = (id: string) => {
    const { [id]: _, ...rest } = set;
    put(rest);
  };
  const warnings = hardToRead((t) => set[t] ?? defaults[t] ?? "");
  const changed = Object.keys(set).length;

  return (
    <div className="color-editor">
      <div className="row">
        <div className="segmented" role="group" aria-label="Which appearance to change">
          {(["light", "dark"] as const).map((m) => (
            <button key={m} type="button" className={mode === m ? "on" : ""} aria-pressed={mode === m} onClick={() => setMode(m)}>
              {m === "light" ? "Light" : "Dark"}
            </button>
          ))}
        </div>
        <button type="button" onClick={() => EngineService.OpenColors().catch(onError)}>
          Open colors.json
        </button>
        <button type="button" disabled={changed === 0} onClick={() => put({})}>
          Reset {mode} colors
        </button>
      </div>
      {other && <p className="muted">The app is showing its {mode === "dark" ? "light" : "dark"} appearance, so these show when it switches to {mode}.</p>}
      {COLOR_GROUPS.map((g) => (
        <fieldset key={g.title} className="color-group">
          <legend>{g.title}</legend>
          {g.tokens.map((t) => {
            // Links and focus follow the accent until they are set themselves.
            const follows = (t.id === "link" || t.id === "focus") && !!set.accent;
            return (
              <ColorRow
                key={`${mode}-${t.id}`}
                id={t.id}
                label={t.label}
                value={set[t.id]}
                fallback={follows ? set.accent : (defaults[t.id] ?? "")}
                placeholder={follows ? "Follows Buttons" : undefined}
                onSet={(v) => setToken(t.id, v)}
                onReset={() => reset(t.id)}
              />
            );
          })}
        </fieldset>
      ))}
      {warnings.length > 0 && (
        <p role="status" className="color-warning">
          Hard to read (4.5:1 is the usual minimum): {warnings.join("; ")}.
        </p>
      )}
    </div>
  );
}

interface RowProps {
  id: string;
  label: string;
  value?: string;
  fallback: string;
  placeholder?: string;
  onSet: (v: string) => void;
  onReset: () => void;
}

function ColorRow({ id, label, value, fallback, placeholder, onSet, onReset }: RowProps) {
  // What is being typed, while it is not a color yet.
  const [draft, setDraft] = useState<string | null>(null);
  const shown = value ?? fallback;
  const typing = draft !== null && !isColor(draft);
  const text = typing ? draft : (value ?? "");
  const bad = typing && draft !== "";
  return (
    <div className="color-row" title={`--${id}`}>
      <input
        type="color"
        aria-label={`${label} color`}
        value={pickerValue(shown) || "#000000"}
        onChange={(e) => {
          setDraft(null);
          onSet(e.target.value);
        }}
      />
      <span className="color-label">{label}</span>
      <input
        type="text"
        className="color-text"
        aria-label={`${label} value`}
        aria-invalid={bad || undefined}
        spellCheck={false}
        placeholder={placeholder ?? fallback}
        value={text}
        onChange={(e) => {
          const v = e.target.value.trim();
          setDraft(v);
          if (isColor(v)) onSet(v);
          else if (v === "") onReset();
        }}
        onBlur={() => setDraft(null)}
      />
      <button type="button" className="link color-reset" disabled={value === undefined} aria-label={`Reset ${label}`} onClick={onReset}>
        Reset
      </button>
    </div>
  );
}
