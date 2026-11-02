// How new sessions inherit a token optimizer (HRN-019), for what the forms say;
// the engine decides the same way.

export const OPTIMIZER_OFF = "off";

// inheritedOptimizer is what a new session gets when nothing is chosen for it:
// the profile's optimizer, then the default in Settings.
export function inheritedOptimizer(profileOptimizer: string | undefined, settingsDefault: string | undefined): string {
  const id = profileOptimizer || settingsDefault || "";
  return id === OPTIMIZER_OFF ? "" : id;
}

// inheritedLabel is the form's first choice, saying where the value comes from.
export function inheritedLabel(profileOptimizer: string | undefined, settingsDefault: string | undefined, list: { id: string; name: string }[]): string {
  const id = inheritedOptimizer(profileOptimizer, settingsDefault);
  const name = id ? (list.find((o) => o.id === id)?.name ?? id) : "Off";
  return `${profileOptimizer ? "From the profile" : "From Settings"} (${name})`;
}
