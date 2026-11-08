// The demo (GUI-045) is the session on the engine's tour provider, which no other
// session can have; its "demo" tag is only for the eye, and a user may tag a session
// of their own "demo". The banner goes by the session's provider, the same for any
// harness, so nothing here special-cases one.
export const TOUR_PROVIDER = "freeloop-tour";

export function isDemo(session: { providerId?: string | null; harnessId?: string | null } | null | undefined): boolean {
  return session?.providerId === TOUR_PROVIDER && session?.harnessId === "builtin";
}
