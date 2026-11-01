import { useState, type KeyboardEvent } from "react";
import { bindingProblem, comboOf, keyLabel, SHORTCUTS, type Bindings } from "./shortcuts.ts";

// ShortcutsEditor changes the app's own shortcuts (SET-006). A row listens while
// its button is pressed; Escape leaves it as it was.

interface Props {
  value: Bindings;
  onChange: (next: Bindings) => void;
}

export default function ShortcutsEditor({ value, onChange }: Props) {
  const [listening, setListening] = useState<string | null>(null);
  const [problem, setProblem] = useState<{ id: string; text: string } | null>(null);
  const take = (id: string, letter: boolean | undefined, e: KeyboardEvent) => {
    if (listening !== id) return;
    // Tab still moves on, so the keyboard cannot get stuck here.
    if (e.key === "Tab") {
      setListening(null);
      return;
    }
    e.preventDefault();
    e.stopPropagation();
    if (e.key === "Escape") {
      setListening(null);
      setProblem(null);
      return;
    }
    const combo = comboOf(e);
    if (!combo) return; // a modifier alone: wait for the key
    const why = bindingProblem(id, combo, value);
    if (why) {
      setProblem({ id, text: why });
      return;
    }
    const key = letter ? combo.toLowerCase() : combo;
    const next = { ...value };
    if (SHORTCUTS.find((s) => s.id === id)?.key === key) delete next[id];
    else next[id] = key;
    onChange(next);
    setProblem(null);
    setListening(null);
  };
  return (
    <ul className="shortcut-list">
      {SHORTCUTS.map((s) => {
        const current = value[s.id] || s.key;
        return (
          <li key={s.id}>
            <span>{s.label}</span>
            <span className="row">
              <button
                type="button"
                className={listening === s.id ? "shortcut-key shortcut-listening" : "shortcut-key"}
                aria-label={`Change ${s.label}, now ${keyLabel(current)}`}
                onClick={() => {
                  setListening(s.id);
                  setProblem(null);
                }}
                onBlur={() => listening === s.id && setListening(null)}
                onKeyDown={(e) => take(s.id, s.letter, e)}
              >
                {listening === s.id ? "Press a key…" : keyLabel(current)}
              </button>
              {value[s.id] && (
                <button
                  type="button"
                  className="link"
                  aria-label={`Reset ${s.label}`}
                  onClick={() => {
                    const next = { ...value };
                    delete next[s.id];
                    onChange(next);
                  }}
                >
                  Reset
                </button>
              )}
            </span>
            {problem?.id === s.id && (
              <p className="error-text" role="alert">
                {problem.text}
              </p>
            )}
          </li>
        );
      })}
    </ul>
  );
}
