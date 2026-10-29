// Model lists for the new-session form (PRV-003): favorites first, even
// when the endpoint does not list them, then the rest without hidden ones
// unless they are asked for.
export function orderModels(ids: readonly string[], favorites: readonly string[], hidden: readonly string[], showHidden: boolean): string[] {
  const fav = new Set(favorites);
  const hide = new Set(hidden);
  const rest = ids.filter((id) => !fav.has(id) && (showHidden || !hide.has(id)));
  return [...favorites, ...new Set(rest)];
}
