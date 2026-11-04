import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { menuPosition, nextMenuItem } from "./menu.ts";

export interface MenuItem {
  label: string;
  run: () => void;
  danger?: boolean;
  // A line above it.
  apart?: boolean;
}

// ContextMenu is a menu at the pointer. It closes on Escape, on a click elsewhere and
// when the window moves on, and gives the keyboard back to where it was.
export default function ContextMenu({ x, y, label, items, onClose }: { x: number; y: number; label: string; items: MenuItem[]; onClose: () => void }) {
  const box = useRef<HTMLDivElement>(null);
  const [place, setPlace] = useState({ left: x, top: y });
  useLayoutEffect(() => {
    const el = box.current;
    if (!el) return;
    setPlace(menuPosition(x, y, el.offsetWidth, el.offsetHeight, window.innerWidth, window.innerHeight));
    el.querySelector("button")?.focus();
  }, [x, y]);
  useEffect(() => {
    const back = document.activeElement instanceof HTMLElement && !box.current?.contains(document.activeElement) ? document.activeElement : null;
    const outside = (e: Event) => {
      if (!(e.target instanceof Node) || !box.current?.contains(e.target)) onClose();
    };
    window.addEventListener("pointerdown", outside, true);
    window.addEventListener("blur", onClose);
    window.addEventListener("resize", onClose);
    window.addEventListener("scroll", outside, true);
    return () => {
      window.removeEventListener("pointerdown", outside, true);
      window.removeEventListener("blur", onClose);
      window.removeEventListener("resize", onClose);
      window.removeEventListener("scroll", outside, true);
      back?.focus?.();
    };
  }, []);
  return (
    <div
      ref={box}
      className="context-menu"
      role="menu"
      aria-label={label}
      style={place}
      onContextMenu={(e) => e.preventDefault()}
      onKeyDown={(e) => {
        if (e.key === "Escape" || e.key === "Tab") {
          e.preventDefault();
          e.stopPropagation();
          onClose();
          return;
        }
        const buttons = [...e.currentTarget.querySelectorAll("button")];
        const next = nextMenuItem(e.key, buttons.indexOf(document.activeElement as HTMLButtonElement), buttons.length);
        if (next < 0) return;
        e.preventDefault();
        buttons[next].focus();
      }}
    >
      {items.map((item) => (
        <div key={item.label} className="context-menu-item">
          {item.apart && <hr />}
          <button
            role="menuitem"
            className={item.danger ? "danger" : undefined}
            onClick={() => {
              onClose();
              item.run();
            }}
          >
            {item.label}
          </button>
        </div>
      ))}
    </div>
  );
}
