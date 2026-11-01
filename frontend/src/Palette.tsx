import { createContext, useContext, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { rank, type PaletteItem } from "./palette.ts";

// The command palette (GUI-014): ⌘K opens it over the window. App supplies
// the items it owns (pages, sessions, filters); views add the actions they
// show while they are on screen with usePaletteActions, so whatever can be
// clicked can also be run from here.

type Source = () => PaletteItem[];

export interface PaletteRegistry {
  register: (source: Source) => () => void;
}

export const PaletteContext = createContext<PaletteRegistry | null>(null);

// usePaletteActions offers items while the calling component is mounted.
// The latest items are read when the palette opens.
export function usePaletteActions(items: PaletteItem[]) {
  const registry = useContext(PaletteContext);
  const latest = useRef(items);
  latest.current = items;
  useEffect(() => registry?.register(() => latest.current), [registry]);
}

// useRegistry is App's side: the registry and a reader of every source.
export function useRegistry(): { registry: PaletteRegistry; collect: () => PaletteItem[] } {
  const sources = useRef(new Set<Source>());
  return useMemo(
    () => ({
      registry: {
        register: (s: Source) => {
          sources.current.add(s);
          return () => void sources.current.delete(s);
        },
      },
      collect: () => [...sources.current].flatMap((s) => s()),
    }),
    [],
  );
}

export function Palette({ items, onClose }: { items: PaletteItem[]; onClose: () => void }) {
  const [query, setQuery] = useState("");
  // Nothing is picked until the user types or moves, so ⌘K then a reflex
  // Enter runs nothing, an approval least of all.
  const [pick, setPick] = useState(-1);
  const shown = useMemo(() => rank(items, query), [items, query]);
  const list = useRef<HTMLUListElement>(null);
  const opener = useRef(document.activeElement as HTMLElement | null);

  useEffect(() => setPick(query.trim() ? 0 : -1), [query]);
  useEffect(() => {
    list.current?.querySelector(`[data-index="${pick}"]`)?.scrollIntoView({ block: "nearest" });
  }, [pick]);

  const close = (restore = true) => {
    onClose();
    if (restore) setTimeout(() => opener.current?.focus?.(), 0);
  };
  const run = (item: PaletteItem | undefined) => {
    if (!item) return;
    close(false);
    // After the palette is gone, so the action can move focus.
    setTimeout(item.run, 0);
  };
  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    const n = shown.length;
    if (e.key === "Escape") {
      e.preventDefault();
      close();
    } else if (e.key === "Tab") {
      // The dialog's only stop: the page behind it is out of reach.
      e.preventDefault();
    } else if (e.key === "ArrowDown" || (e.ctrlKey && e.key === "n")) {
      e.preventDefault();
      if (n) setPick((p) => (p + 1) % n);
    } else if (e.key === "ArrowUp" || (e.ctrlKey && e.key === "p")) {
      e.preventDefault();
      if (n) setPick((p) => (p < 0 ? n - 1 : (p + n - 1) % n));
    } else if (e.key === "Enter" && !e.nativeEvent.isComposing) {
      e.preventDefault();
      run(shown[pick]);
    }
  };

  return (
    <div className="palette-backdrop" onMouseDown={(e) => e.target === e.currentTarget && close()}>
      <div className="palette" role="dialog" aria-modal="true" aria-label="Command palette">
        <input
          autoFocus
          role="combobox"
          aria-expanded="true"
          aria-controls="palette-list"
          aria-activedescendant={shown[pick] ? `palette-${pick}` : undefined}
          aria-autocomplete="list"
          placeholder="Go to a session or page, or run an action"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={onKeyDown}
        />
        <ul id="palette-list" role="listbox" ref={list} aria-label="Results">
          {shown.map((item, i) => (
            <li
              key={item.id}
              id={`palette-${i}`}
              data-index={i}
              role="option"
              aria-selected={i === pick}
              className={i === pick ? "palette-item palette-on" : "palette-item"}
              onMouseMove={() => i !== pick && setPick(i)}
              onClick={() => run(item)}
            >
              <span className="palette-group">{item.group}</span>
              <span className="palette-title">{item.title}</span>
              {item.detail && <span className="palette-detail muted">{item.detail}</span>}
              {item.shortcut && <kbd className="palette-shortcut" aria-hidden="true">{item.shortcut}</kbd>}
            </li>
          ))}
          {shown.length === 0 && <li className="muted palette-empty">Nothing matches.</li>}
        </ul>
      </div>
    </div>
  );
}
