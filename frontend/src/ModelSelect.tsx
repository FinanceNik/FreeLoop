import { useState } from "react";
import { modelGroups, namedModel, type ChoiceLike } from "./modelChoice.ts";
import { t } from "./i18n.ts";

// A model chosen from the choices grouped by provider, with "Other model on …" per
// provider to name one by hand (WFL-011): the AFK dialog's pickers and the rows of a
// model set use it. The empty value is emptyLabel, such as the profile's own model.

const OTHER = "\u0000other:";

export default function ModelSelect({ choices, custom, value, emptyLabel, disabled, onChange }: { choices: readonly ChoiceLike[]; custom: boolean; value: string; emptyLabel: string; disabled?: boolean; onChange: (v: string) => void }) {
  // The provider a model is being named on, while it is.
  const [naming, setNaming] = useState("");
  const [name, setName] = useState("");
  const groups = modelGroups(choices);
  const listed = choices.some((c) => c.value === value);
  const provider = (v: string) => (v.includes("\n") ? v.split("\n")[0] : "");
  const selected = naming ? OTHER + naming : value && !listed ? OTHER + provider(value) : value;
  return (
    <>
      <select
        value={selected}
        disabled={disabled || choices.length === 0}
        onChange={(e) => {
          const v = e.target.value;
          if (v.startsWith(OTHER)) {
            setNaming(v.slice(OTHER.length));
            setName("");
            onChange("");
            return;
          }
          setNaming("");
          onChange(v);
        }}
      >
        <option value="">{emptyLabel}</option>
        {groups.map((g) => {
          const items = g.items.map((c) => (
            <option key={c.value} value={c.value}>
              {c.name}
            </option>
          ));
          if (custom && g.provider) {
            items.push(
              <option key={OTHER + g.provider} value={OTHER + g.provider}>
                {t("afk.model.other", { provider: g.group })}
              </option>,
            );
          }
          return g.group ? (
            <optgroup key={g.group} label={g.group}>
              {items}
            </optgroup>
          ) : (
            items
          );
        })}
        {/* A value whose provider is not among the groups, such as one that is gone. */}
        {value && !listed && !groups.some((g) => g.provider === provider(value)) && <option value={OTHER + provider(value)}>{value.split("\n").slice(1).join("") || value}</option>}
      </select>
      {(naming || (value && !listed)) && (
        <input
          aria-label={t("afk.model.otherName")}
          placeholder={t("afk.model.otherName")}
          value={naming ? name : value.split("\n").slice(1).join("")}
          onChange={(e) => {
            const p = naming || provider(value);
            setNaming(p);
            setName(e.target.value);
            onChange(namedModel(p, e.target.value));
          }}
        />
      )}
    </>
  );
}
