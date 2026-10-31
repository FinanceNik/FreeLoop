import { useState } from "react";
import { MAX_TAGS, normalizeTags } from "./filters.ts";

// Tags edits a session's tags (SES-013): each one a chip that can be
// removed, and a field that adds the tags typed into it, separated by
// commas. The engine cleans what it is given and returns the tags kept.
export default function Tags({
  tags,
  known,
  adding,
  onAdding,
  onChange,
}: {
  tags: string[];
  known: string[];
  adding: boolean;
  onAdding: (open: boolean) => void;
  onChange: (tags: string[]) => void;
}) {
  const [draft, setDraft] = useState("");
  const add = () => {
    const next = normalizeTags([...tags, ...draft.split(",")]);
    setDraft("");
    if (next.join(" ") !== tags.join(" ")) onChange(next.slice(0, MAX_TAGS));
  };
  if (tags.length === 0 && !adding) return null;
  return (
    <div className="tags" role="group" aria-label="Tags">
      {tags.map((t) => (
        <span key={t} className="tag tag-editable">
          #{t}
          <button aria-label={`Remove the tag ${t}`} onClick={() => onChange(tags.filter((x) => x !== t))}>
            ×
          </button>
        </span>
      ))}
      {adding ? (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            add();
          }}
        >
          <input
            autoFocus
            className="tag-input"
            aria-label="New tag"
            placeholder="tag, another"
            list="known-tags"
            value={draft}
            maxLength={200}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                setDraft("");
                onAdding(false);
              }
            }}
            onBlur={() => {
              if (draft.trim()) add();
              onAdding(false);
            }}
          />
          <datalist id="known-tags">
            {known
              .filter((t) => !tags.includes(t))
              .map((t) => (
                <option key={t} value={t} />
              ))}
          </datalist>
        </form>
      ) : (
        tags.length < MAX_TAGS && (
          <button className="link tag-add" onClick={() => onAdding(true)}>
            + Tag
          </button>
        )
      )}
    </div>
  );
}
