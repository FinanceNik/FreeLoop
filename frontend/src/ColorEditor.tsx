import { useMemo, useState } from "react";
import { t } from "./i18n.ts";
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
        <div className="segmented" role="group" aria-label={t("colors.which")}>
          {(["light", "dark"] as const).map((m) => (
            <button key={m} type="button" className={mode === m ? "on" : ""} aria-pressed={mode === m} onClick={() => setMode(m)}>
              {t(m === "light" ? "settings.theme.light" : "settings.theme.dark")}
            </button>
          ))}
        </div>
        <button type="button" onClick={() => EngineService.OpenColors().catch(onError)}>
          {t("colors.open")}
        </button>
        <button type="button" disabled={changed === 0} onClick={() => put({})}>
          {t(mode === "light" ? "colors.resetLight" : "colors.resetDark")}
        </button>
      </div>
      {other && <p className="muted">{t(mode === "dark" ? "colors.showingLight" : "colors.showingDark")}</p>}
      {COLOR_GROUPS.map((g) => (
        <fieldset key={g.title} className="color-group">
          <legend>{t(g.title)}</legend>
          {g.tokens.map((token) => {
            // Links and focus follow the accent until they are set themselves.
            const follows = (token.id === "link" || token.id === "focus") && !!set.accent;
            return (
              <ColorRow
                key={`${mode}-${token.id}`}
                id={token.id}
                label={t(token.label)}
                value={set[token.id]}
                fallback={follows ? set.accent : (defaults[token.id] ?? "")}
                placeholder={follows ? t("colors.followsButtons") : undefined}
                onSet={(v) => setToken(token.id, v)}
                onReset={() => reset(token.id)}
              />
            );
          })}
        </fieldset>
      ))}
      {warnings.length > 0 && (
        <p role="status" className="color-warning">
          {t("colors.hardToRead", { pairs: warnings.join("; ") })}
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
        aria-label={t("colors.pick", { label })}
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
        aria-label={t("colors.value", { label })}
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
      <button type="button" className="link color-reset" disabled={value === undefined} aria-label={t("common.resetNamed", { name: label })} onClick={onReset}>
        {t("common.reset")}
      </button>
    </div>
  );
}
