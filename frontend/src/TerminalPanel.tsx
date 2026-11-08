import { useEffect, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { EngineService, TERMINAL_EVENT_NAME, type Session, type TerminalInfo, type TerminalOutput } from "./api.ts";
import { after, bytesOf, fitSize } from "./terminal.ts";
import { terminalClipboard } from "./platform.ts";

// TerminalPanel is the Terminal tab of the right panel (GUI-012): shells in
// the session's environment, a container for a Docker session. The shells
// live in the engine, so they keep running while the tab is closed; opening
// it again shows what they printed meanwhile. Typing here is the user's own
// action and is not recorded in the session.

interface Props {
  session: Session;
  onError: (err: unknown) => void;
}

export default function TerminalPanel({ session, onError }: Props) {
  const [terms, setTerms] = useState<TerminalInfo[] | null>(null);
  const [current, setCurrent] = useState<string>("");
  const archived = session.status === "archived";

  const open = () =>
    EngineService.OpenTerminal(session.id, 80, 24)
      .then((t) => {
        setTerms((list) => [...(list ?? []), t]);
        setCurrent(t.id);
      })
      .catch(onError);

  // Read again after an archive, which ends the session's shells.
  useEffect(() => {
    if (archived) {
      setTerms(null);
      setCurrent("");
      return;
    }
    EngineService.SessionTerminals(session.id)
      .then((list) => {
        setTerms(list ?? []);
        if (list?.length) setCurrent(list[list.length - 1].id);
        else open();
      })
      .catch(onError);
  }, [session.id, archived]);

  const close = (id: string) => {
    EngineService.CloseTerminal(id).catch(() => {});
    const rest = (terms ?? []).filter((t) => t.id !== id);
    setTerms(rest);
    if (current === id) setCurrent(rest.length ? rest[rest.length - 1].id : "");
  };

  if (archived) return <p className="muted">Unarchive the session to open a terminal in it.</p>;
  if (!terms) return <p className="muted">Starting…</p>;
  const shown = terms.find((t) => t.id === current);
  return (
    <div className="terminal-panel">
      <div className="row terminal-tabs" role="group" aria-label="Terminals">
        {terms.map((t, i) => (
          <span key={t.id} className={t.id === current ? "terminal-tab terminal-tab-on" : "terminal-tab"}>
            <button aria-current={t.id === current ? "true" : undefined} className="link" onClick={() => setCurrent(t.id)}>
              Shell {i + 1}
            </button>
            <button className="link" aria-label={`Close shell ${i + 1}`} title="Close this shell" onClick={() => close(t.id)}>
              ×
            </button>
          </span>
        ))}
        <button className="link" onClick={open} title="Open another shell">
          + New
        </button>
      </div>
      {shown ? (
        <>
          <p className="muted terminal-where">
            <code>{shown.where}</code> · Shift+Esc leaves the terminal
          </p>
          <TerminalView key={shown.id} id={shown.id} onError={onError} />
        </>
      ) : (
        <p className="muted">No shell is open.</p>
      )}
    </div>
  );
}

// cellSize reads the size of one character cell from xterm's renderer; the
// public API does not offer it.
function cellSize(term: Terminal): { width: number; height: number } {
  const dims = (term as unknown as { _core?: { _renderService?: { dimensions?: { css?: { cell?: { width: number; height: number } } } } } })._core?._renderService
    ?.dimensions?.css?.cell;
  return { width: dims?.width ?? 0, height: dims?.height ?? 0 };
}

function themeColors() {
  const css = getComputedStyle(document.documentElement);
  const v = (name: string) => css.getPropertyValue(name).trim();
  return { background: v("--bg-card") || "#1b1b1a", foreground: v("--text") || "#ececea", cursor: v("--text") || "#ececea" };
}

// The code font in force (GUI-025): xterm takes its font as a value, not from
// the page's style sheet.
function codeFont() {
  return getComputedStyle(document.documentElement).getPropertyValue("--mono").trim() || "monospace";
}

function TerminalView({ id, onError }: { id: string; onError: (err: unknown) => void }) {
  const box = useRef<HTMLDivElement>(null);
  const [exited, setExited] = useState<number | null>(null);

  useEffect(() => {
    const term = new Terminal({
      fontFamily: codeFont(),
      // Two points under the app's text, as it was at the default of 14.
      fontSize: (parseFloat(getComputedStyle(document.documentElement).fontSize) || 14) - 2,
      cursorBlink: true,
      // Lets a screen reader read the output and the line being typed.
      screenReaderMode: true,
      scrollback: 5000,
      theme: themeColors(),
    });
    term.open(box.current!);
    // A shell takes Tab, so the keyboard needs another way out (NFR-A11Y-001).
    term.attachCustomKeyEventHandler((e) => {
      // The system's copy and paste go to the browser, which hands them to xterm.
      if (terminalClipboard(e, term.hasSelection())) return false;
      if (e.key !== "Escape" || !e.shiftKey) return true;
      if (e.type === "keydown") document.querySelector<HTMLElement>('.panel-tablist [role="tab"][aria-selected="true"]')?.focus();
      return false;
    });
    let size = { cols: 0, rows: 0 };
    const fit = () => {
      const el = box.current;
      const cell = cellSize(term);
      if (!el || !cell.width) return;
      const next = fitSize(el.clientWidth - 8, el.clientHeight - 4, cell.width, cell.height);
      if (next.cols === size.cols && next.rows === size.rows) return;
      size = next;
      term.resize(next.cols, next.rows);
      EngineService.ResizeTerminal(id, next.cols, next.rows).catch(() => {});
    };
    const observer = new ResizeObserver(() => fit());
    // The text size and the code font can change while the terminal is open.
    const rootStyle = new MutationObserver(() => {
      term.options.fontSize = (parseFloat(getComputedStyle(document.documentElement).fontSize) || 14) - 2;
      term.options.fontFamily = codeFont();
      fit();
    });
    rootStyle.observe(document.documentElement, { attributes: true, attributeFilter: ["style"] });
    observer.observe(box.current!);

    // What it printed so far, then what it prints from now on; chunks that
    // arrive while the screen loads wait for it.
    let shownTo = -1;
    const early: TerminalOutput[] = [];
    const show = (out: TerminalOutput) => {
      const data = bytesOf(out.data);
      const fresh = after(data, out.offset, shownTo);
      if (fresh.length) term.write(fresh);
      shownTo = Math.max(shownTo, out.offset + data.length);
      if (out.exited) setExited(out.exitCode ?? 0);
    };
    const off = Events.On(TERMINAL_EVENT_NAME, ({ data }) => {
      const out = data as TerminalOutput;
      if (out.id !== id) return;
      if (shownTo >= 0) show(out);
      else early.push(out);
    });
    EngineService.TerminalScreen(id)
      .then((scr) => {
        shownTo = scr.offset;
        show(scr);
        for (const out of early) show(out);
        fit();
        term.focus();
      })
      .catch(onError);
    const input = term.onData((d) => EngineService.WriteTerminal(id, d).catch(() => {}));
    return () => {
      off();
      input.dispose();
      observer.disconnect();
      rootStyle.disconnect();
      term.dispose();
    };
  }, [id]);

  return (
    <>
      <div className="terminal-box" ref={box} role="group" aria-label="Terminal" />
      {exited !== null && <p className="muted">The shell exited{exited ? ` with code ${exited}` : ""}.</p>}
    </>
  );
}
