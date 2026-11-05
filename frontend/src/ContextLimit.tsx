import { useEffect, useState, type KeyboardEvent } from "react";
import { EngineService } from "./api.ts";
import { parseLimit } from "./contextMeter.ts";

// ContextLimit sets how large a model's context may be, for a model nothing reports it
// for (GUI-036). Without one there is no share to show, no warning and no compaction.
export default function ContextLimit({ providerId, model, onChanged, onError }: { providerId: string; model: string; onChanged?: () => void; onError: (err: unknown) => void }) {
  const [saved, setSaved] = useState("");
  const [text, setText] = useState("");
  const [detected, setDetected] = useState(0);
  const [known, setKnown] = useState(false);
  const [done, setDone] = useState(false);

  const load = () =>
    EngineService.ModelCapabilities(providerId, model, false)
      .then((c) => {
        const own = c?.contextLimitSet ? String(c.contextWindow) : "";
        setSaved(own);
        setText(own);
        setDetected(c?.contextLimitSet ? 0 : (c?.contextWindow ?? 0));
        setKnown((c?.contextWindow ?? 0) > 0);
      })
      .catch(() => setKnown(false));
  useEffect(() => {
    setDone(false);
    load();
  }, [providerId, model]);

  const limit = parseLimit(text);
  const save = () => {
    if (limit === null) return;
    EngineService.SetContextLimit(providerId, model, limit)
      .then(load)
      .then(() => {
        setDone(true);
        onChanged?.();
      })
      .catch(onError);
  };
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key !== "Enter") return;
    // In the new-session form, Enter here must not create the session.
    e.preventDefault();
    if (text !== saved) save();
  };
  return (
    <div className="context-limit">
      <input
        aria-label={`Context limit of ${model}, in tokens`}
        inputMode="numeric"
        spellCheck={false}
        value={text}
        placeholder={detected > 0 ? `${detected.toLocaleString()} (detected)` : "Not known"}
        onChange={(e) => {
          setText(e.target.value);
          setDone(false);
        }}
        onKeyDown={onKeyDown}
      />
      {text !== saved && (
        <button type="button" disabled={limit === null} onClick={save}>
          Save
        </button>
      )}
      {done && text === saved && <span className="ok">Saved. It applies from the next message.</span>}
      {!known && !done && <span className="muted">Add the model's limit, such as 200k, to get a share, a warning and automatic compaction.</span>}
    </div>
  );
}
