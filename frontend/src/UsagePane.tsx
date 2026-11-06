import { useEffect, useState, type DragEvent, type KeyboardEvent } from "react";
import { ChevronIcon, CloseIcon, ExpandIcon } from "./icons.tsx";
import Limits from "./Limits.tsx";
import LiveUsage from "./LiveUsage.tsx";
import PaneRange from "./PaneRange.tsx";
import UsageOverview from "./UsageOverview.tsx";
import { cycleView, PANE_VIEWS, viewIndex } from "./usagePane.ts";

// The usage pane (GUI-023, GUI-038): one view at a time, which the arrows either side of
// its name step through. Only the view shown is mounted, so the others ask for nothing.

type Named = { id: string; name: string };
const VIEW_KEY = "freeloop.usagePaneView";

function storedView(): number {
  try {
    return viewIndex(localStorage.getItem(VIEW_KEY));
  } catch {
    return 0;
  }
}

interface Props {
  draggable: boolean;
  onDragStart: (e: DragEvent) => void;
  onUsagePage: () => void;
  onClose: () => void;
  projects: readonly Named[];
  providers: readonly Named[];
  harnesses: readonly Named[];
  onError: (err: unknown) => void;
}

export default function UsagePane({ draggable, onDragStart, onUsagePage, onClose, projects, providers, harnesses, onError }: Props) {
  const [at, setAt] = useState(storedView);
  useEffect(() => {
    try {
      localStorage.setItem(VIEW_KEY, PANE_VIEWS[at].id);
    } catch {
      // Only a convenience: the pane opens on its first view next time.
    }
  }, [at]);
  const n = PANE_VIEWS.length;
  const go = (step: number) => setAt((i) => cycleView(i, step, n));
  const previous = PANE_VIEWS[cycleView(at, -1, n)].label;
  const next = PANE_VIEWS[cycleView(at, 1, n)].label;
  const view = PANE_VIEWS[at].id;
  // The arrow keys move between views from anywhere in the header; with modifiers they
  // are the workspace's, which moves between panes.
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.ctrlKey || e.shiftKey || e.altKey || e.metaKey || (e.key !== "ArrowLeft" && e.key !== "ArrowRight")) return;
    e.preventDefault();
    go(e.key === "ArrowLeft" ? -1 : 1);
  };
  return (
    <div className="pane-usage" data-view={view}>
      <div className="pane-usage-head" draggable={draggable} onDragStart={onDragStart} onKeyDown={onKeyDown}>
        <div className="usage-views" role="group" aria-label="Usage views">
          <button className="icon-button usage-view-step usage-view-previous" onClick={() => go(-1)} aria-label={`Previous view: ${previous}`} title={previous}>
            <ChevronIcon />
          </button>
          <h2>
            Usage · <span className="usage-view-name">{PANE_VIEWS[at].label}</span>
          </h2>
          <span className="muted usage-view-count" aria-label={`View ${at + 1} of ${n}`}>
            {at + 1}/{n}
          </span>
          <button className="icon-button usage-view-step" onClick={() => go(1)} aria-label={`Next view: ${next}`} title={next}>
            <ChevronIcon />
          </button>
        </div>
        <span className="spacer" />
        <button className="icon-button" aria-label="Full usage page" title="Full usage page" onClick={onUsagePage}>
          <ExpandIcon />
        </button>
        <button className="icon-button" onClick={onClose} aria-label="Close this pane" title="Close this pane">
          <CloseIcon />
        </button>
      </div>
      {view === "recent" && (
        <>
          <Limits compact onError={onError} />
          <LiveUsage compact projects={projects} providers={providers} harnesses={harnesses} onError={onError} />
        </>
      )}
      {view === "year" && <UsageOverview compact cacheReads onError={onError} />}
      {(view === "tokens" || view === "models") && <PaneRange key={view} view={view} providers={providers} harnesses={harnesses} onError={onError} />}
    </div>
  );
}
