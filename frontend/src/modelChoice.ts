// Choosing a model for a run or a model set (WFL-011): the choices grouped by their
// provider, a model named by hand, and a chosen value in words. A built-in agent's
// value is its provider's ID and the model, a line apart. Pure, so modelChoice.test.ts
// covers it.
import { t } from "./i18n.ts";

export interface ChoiceLike {
  value: string;
  name: string;
  group: string;
}

export interface ModelGroup {
  group: string;
  // The provider's ID, which a model named by hand goes to; empty for an external
  // agent's choices, whose values name no provider.
  provider: string;
  items: { value: string; name: string }[];
}

const SEP = "\n";

function split(value: string): [string, string] {
  const i = value.indexOf(SEP);
  return i < 0 ? ["", value] : [value.slice(0, i), value.slice(i + 1)];
}

// modelGroups groups the choices by provider, in the order the engine gave them (the
// profile's own provider first).
export function modelGroups(choices: readonly ChoiceLike[]): ModelGroup[] {
  const out: ModelGroup[] = [];
  for (const c of choices) {
    const [provider, model] = split(c.value);
    let g = out.find((x) => x.group === c.group);
    if (!g) {
      g = { group: c.group, provider, items: [] };
      out.push(g);
    }
    g.items.push({ value: c.value, name: c.name || model });
  }
  return out;
}

// namedModel is the value of a model typed by hand on a provider, or "" when the name
// is blank or not one line.
export function namedModel(provider: string, name: string): string {
  const n = name.trim();
  return !n || /[\r\n]/.test(n) ? "" : provider + SEP + n;
}

// modelLabel is a value as a person reads it: the model, with its provider's name when
// it is not among the choices.
export function modelLabel(choices: readonly ChoiceLike[], value: string): string {
  if (!value) return "";
  const c = choices.find((x) => x.value === value);
  const [provider, model] = split(value);
  if (c) return c.name || model;
  const group = choices.find((x) => split(x.value)[0] === provider)?.group || provider;
  return group ? `${model} (${group})` : model;
}

// keptEffort is the effort still chosen once the model's efforts are known: the same
// level when the model offers it, else the profile's (WFL-012).
export function keptEffort(choices: readonly { value: string }[], chosen: string): string {
  return chosen && choices.some((c) => c.value === chosen) ? chosen : "";
}

// fromSetNote says where a run's model comes from (WFL-014): the workflow's model set,
// or a change for this run only; nothing when the set fixes no model there.
export function fromSetNote(setValue: string, value: string, setName: string): string {
  if (!setValue || !setName) return "";
  return value === setValue ? t("afk.model.fromSet", { name: setName }) : t("afk.model.changedForRun");
}

// modelOffered reports whether a profile can have value, as the engine decides when a
// run starts: one of its choices, or a model named on a provider the choices come from.
export function modelOffered(models: { choices: readonly ChoiceLike[]; custom: boolean }, value: string): boolean {
  if (!value || models.choices.some((c) => c.value === value)) return true;
  const [provider, model] = split(value);
  return models.custom && !!provider && !!model.trim() && models.choices.some((c) => split(c.value)[0] === provider);
}
