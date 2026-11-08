import { useMemo, useState } from "react";
import { CODE_FONTS, fontOnThisMac, isFontName, TEXT_FONTS, type FontChoice } from "./fonts.ts";

// The fonts of Settings → Appearance (GUI-025). It edits ui.textFont and
// ui.codeFont in config.yaml: a change shows at once and reaches the file a
// moment later, and an edit of the file comes back here.

interface Props {
  textFont: string;
  codeFont: string;
  onChange: (fonts: FontChoice) => void;
}

export default function FontEditor({ textFont, codeFont, onChange }: Props) {
  return (
    <div className="font-editor">
      <FontRow label="Text" hint="The interface and messages." choices={TEXT_FONTS} value={textFont} onSet={(v) => onChange({ textFont: v, codeFont })} />
      <FontRow
        label="Code"
        hint="Code blocks, inline code, tool output, diffs and the terminal."
        choices={CODE_FONTS}
        value={codeFont}
        onSet={(v) => onChange({ textFont, codeFont: v })}
      />
    </div>
  );
}

const OTHER = "other";

interface RowProps {
  label: string;
  hint: string;
  choices: string[];
  value: string;
  onSet: (v: string) => void;
}

function FontRow({ label, hint, choices, value, onSet }: RowProps) {
  const present = useMemo(() => choices.filter(fontOnThisMac), [choices]);
  // Other was chosen, with no name typed yet.
  const [naming, setNaming] = useState(false);
  // What is being typed. A name with a space on the end is a name on its way,
  // so the field shows this and not the name in force.
  const [draft, setDraft] = useState<string | null>(null);
  const listed = present.includes(value);
  const typed = naming || draft !== null || (value !== "" && !listed);
  const text = draft ?? value;
  const bad = draft !== null && draft.trim() !== "" && !isFontName(draft.trim());
  const onThisMac = useMemo(() => value === "" || fontOnThisMac(value), [value]);
  const missing = !bad && !onThisMac;
  return (
    <div className="font-row">
      <label>
        {label}
        <select
          value={typed ? OTHER : value}
          onChange={(e) => {
            const v = e.target.value;
            setDraft(null);
            setNaming(v === OTHER);
            if (v !== OTHER) onSet(v);
          }}
        >
          <option value="">The app's own font</option>
          {present.map((f) => (
            <option key={f} value={f}>
              {f}
            </option>
          ))}
          <option value={OTHER}>Other…</option>
        </select>
      </label>
      {typed && (
        <input
          type="text"
          aria-label={`${label} font name`}
          aria-invalid={bad || undefined}
          spellCheck={false}
          autoComplete="off"
          placeholder="Family name, as in Font Book"
          value={text}
          onChange={(e) => {
            const v = e.target.value.trim();
            setDraft(e.target.value);
            if (v === "") onSet("");
            else if (isFontName(v)) onSet(v);
          }}
          onBlur={() => {
            setDraft(null);
            // Listed after all, or nothing typed: the menu says so again.
            setNaming(false);
          }}
        />
      )}
      <span className="muted">{hint}</span>
      {bad && <span role="status" className="font-note">A font name has only letters, digits, spaces and . _ + -, up to 64 characters.</span>}
      {missing && <span role="status" className="font-note">{value} is not on this computer, so the app's own font shows.</span>}
    </div>
  );
}
