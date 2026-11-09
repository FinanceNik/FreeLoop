import { useState } from "react";
import { MAX_TAGS, normalizeTags } from "./filters.ts";
import { t } from "./i18n.ts";

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
    <div className="tags" role="group" aria-label={t("tags.label")}>
      {tags.map((tag) => (
        <span key={tag} className="tag tag-editable">
          #{tag}
          <button aria-label={t("tags.remove", { tag })} onClick={() => onChange(tags.filter((x) => x !== tag))}>
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
            aria-label={t("tags.new")}
            placeholder={t("tags.placeholder")}
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
              .filter((tag) => !tags.includes(tag))
              .map((tag) => (
                <option key={tag} value={tag} />
              ))}
          </datalist>
        </form>
      ) : (
        tags.length < MAX_TAGS && (
          <button className="link tag-add" onClick={() => onAdding(true)}>
            {t("tags.add")}
          </button>
        )
      )}
    </div>
  );
}
