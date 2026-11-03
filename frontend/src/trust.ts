// What the "Trust project" actions do next. The hooks a project would run are
// shown before it is trusted (EXT-005), so not knowing them is not the same as
// there being none: null means the lookup has not answered or failed.
export type TrustStep = "lookup" | "confirm" | "trust";

export function trustStep(hooks: readonly unknown[] | null, ask: boolean): TrustStep {
  if (hooks === null) return "lookup";
  return ask && hooks.length > 0 ? "confirm" : "trust";
}
