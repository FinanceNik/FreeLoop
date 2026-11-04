import { useRef, type CSSProperties, type KeyboardEvent, type PointerEvent } from "react";

// Splitter is a draggable line between two parts of the window (GUI-032). The arrow keys
// move it too, and a double click puts it back.
export default function Splitter({
  label,
  className,
  value,
  min,
  max,
  step,
  fromPointer,
  onChange,
  onReset,
  style,
}: {
  label: string;
  className: string;
  value: number;
  min: number;
  max: number;
  // How far an arrow key moves it, to the right.
  step: number;
  // The value for the pointer at this x.
  fromPointer: (x: number) => number;
  onChange: (value: number) => void;
  onReset: () => void;
  style?: CSSProperties;
}) {
  // The latest of these, for a drag that started in an earlier render.
  const latest = useRef({ fromPointer, onChange });
  latest.current = { fromPointer, onChange };
  // On the window, so the drag goes on wherever the pointer goes.
  const startDrag = () => {
    const move = (e: globalThis.PointerEvent) => latest.current.onChange(latest.current.fromPointer(e.clientX));
    const stop = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", stop);
      window.removeEventListener("pointercancel", stop);
      document.documentElement.classList.remove("resizing");
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", stop);
    window.addEventListener("pointercancel", stop);
    document.documentElement.classList.add("resizing");
  };
  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      aria-valuenow={Math.round(value)}
      aria-valuemin={Math.round(min)}
      aria-valuemax={Math.round(max)}
      tabIndex={0}
      title={`${label}. Double-click to reset.`}
      className={`splitter ${className}`}
      style={style}
      onPointerDown={(e: PointerEvent<HTMLDivElement>) => {
        if (e.button !== 0) return;
        e.preventDefault();
        // Over a preview's frame the page would otherwise stop hearing the pointer.
        e.currentTarget.setPointerCapture(e.pointerId);
        startDrag();
      }}
      onDoubleClick={onReset}
      onKeyDown={(e: KeyboardEvent) => {
        if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
        e.preventDefault();
        e.stopPropagation();
        onChange(value + (e.key === "ArrowRight" ? step : -step));
      }}
    />
  );
}
