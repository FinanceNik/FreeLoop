import { useEffect, type RefObject } from "react";

// useDismiss closes a <details> menu on a click outside it and on Escape, which returns
// focus to its summary, as a Mac menu does.
export function useDismiss(ref: RefObject<HTMLDetailsElement | null>) {
  useEffect(() => {
    const outside = (e: PointerEvent) => {
      const menu = ref.current;
      if (menu?.open && !menu.contains(e.target as Node)) menu.open = false;
    };
    const escape = (e: KeyboardEvent) => {
      const menu = ref.current;
      if (!menu?.open || e.key !== "Escape") return;
      e.preventDefault();
      e.stopPropagation();
      menu.open = false;
      menu.querySelector("summary")?.focus();
    };
    window.addEventListener("pointerdown", outside, true);
    window.addEventListener("keydown", escape, true);
    return () => {
      window.removeEventListener("pointerdown", outside, true);
      window.removeEventListener("keydown", escape, true);
    };
  }, [ref]);
}
