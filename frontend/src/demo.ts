// The demo (GUI-045) is the session the engine tags "demo". The interface goes by
// the tag, never by which harness or provider runs it.
export const DEMO_TAG = "demo";

export function isDemo(session: { tags?: string[] | null } | null | undefined): boolean {
  return !!session?.tags?.includes(DEMO_TAG);
}
