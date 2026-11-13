// What Settings shows about updating the app in place (SET-008), from what the
// engine says. Kept apart from the view so it can be tested without a DOM.

// The fields of the engine's UpdateInfo this reads.
export interface UpdateState {
  newer?: boolean;
  latest?: string;
  canInstall?: boolean;
  whyNot?: string;
  step?: string;
  received?: number;
  total?: number;
  ready?: string;
  installError?: string;
  busy?: { working?: number; waiting?: number } | null;
}

export type UpdateView =
  // Nothing newer is known, so there is nothing to offer.
  | { kind: "none" }
  // A newer release this copy can replace itself with.
  | { kind: "offer"; version: string; failed: string }
  // A newer release this copy cannot install, and why: only its page is offered.
  | { kind: "cannot"; version: string; reason: string }
  | { kind: "downloading"; version: string; percent: number }
  // Downloaded and checked; restarting installs it. busy says work would be interrupted.
  | { kind: "ready"; version: string; busy: boolean; failed: string }
  | { kind: "installing"; version: string };

export function updateView(u: UpdateState | null | undefined): UpdateView {
  if (!u) return { kind: "none" };
  const version = u.ready || u.latest || "";
  const failed = u.installError ?? "";
  switch (u.step) {
    case "downloading": {
      const total = u.total ?? 0;
      const percent = total > 0 ? Math.min(100, Math.floor((100 * (u.received ?? 0)) / total)) : 0;
      return { kind: "downloading", version, percent };
    }
    case "ready":
      return { kind: "ready", version, busy: (u.busy?.working ?? 0) + (u.busy?.waiting ?? 0) > 0, failed };
    case "installing":
      return { kind: "installing", version };
  }
  if (!u.newer) return { kind: "none" };
  if (!u.canInstall) return { kind: "cannot", version, reason: u.whyNot ?? "" };
  return { kind: "offer", version, failed };
}
