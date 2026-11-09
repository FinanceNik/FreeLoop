import { useEffect, useState } from "react";
import { Browser } from "@wailsio/runtime";
import { EngineService, type Session } from "./api.ts";
import { parseAddress } from "./preview.ts";
import { t } from "./i18n.ts";

// PreviewPanel is the Preview tab (GUI-020): the session's running app in a
// frame, by default on its own port. The frame is a page the agent may have
// written, so it runs sandboxed: no popups, no navigating this window, and
// the engine refuses calls from any origin but the app's own.

// The address typed per session, while the app runs.
const addresses = new Map<string, string>();

export default function PreviewPanel({ session, onError }: { session: Session; onError: (err: unknown) => void }) {
  const initial = addresses.get(session.id) ?? (session.portBase > 0 ? String(session.portBase) : "");
  const [text, setText] = useState(initial);
  const [shown, setShown] = useState(initial);
  const [reloads, setReloads] = useState(0);
  const [listening, setListening] = useState<boolean | null>(null);
  const address = parseAddress(shown, window.location.origin);
  const url = "error" in address ? "" : address.url;
  const port = "error" in address ? 0 : address.port;

  useEffect(() => {
    setListening(null);
    if (!port) return;
    let live = true;
    const check = () =>
      Promise.resolve(EngineService.PortOpen(port))
        .then((open) => {
          if (!live) return;
          // A server that just came up gets a fresh load.
          setListening((was) => {
            if (was === false && open) setReloads((n) => n + 1);
            return open;
          });
        })
        .catch(() => {});
    check();
    const timer = setInterval(check, 2500);
    return () => {
      live = false;
      clearInterval(timer);
    };
  }, [port]);

  const go = () => {
    addresses.set(session.id, text);
    setShown(text);
    setReloads((n) => n + 1);
  };

  return (
    <div className="preview-panel">
      <form
        className="row preview-bar"
        onSubmit={(e) => {
          e.preventDefault();
          go();
        }}
      >
        <input aria-label={t("preview.address")} placeholder={t("preview.placeholder")} value={text} onChange={(e) => setText(e.target.value)} />
        <button type="submit">{t("preview.go")}</button>
        <button type="button" disabled={!url} onClick={() => setReloads((n) => n + 1)}>
          {t("preview.reload")}
        </button>
        <button type="button" disabled={!url} onClick={() => Browser.OpenURL(url).catch(onError)}>
          {t("preview.openInBrowser")}
        </button>
      </form>
      {"error" in address && <p className="muted preview-note">{shown ? address.error : t("preview.empty")}</p>}
      {url && listening === false && <p className="muted preview-note">{t("preview.notListening", { port: String(port) })}</p>}
      {url && listening && (
        <iframe
          key={`${url}#${reloads}`}
          className="preview-frame"
          title={t("preview.frameTitle", { url })}
          src={url}
          sandbox="allow-scripts allow-forms allow-same-origin"
          referrerPolicy="no-referrer"
        />
      )}
    </div>
  );
}
