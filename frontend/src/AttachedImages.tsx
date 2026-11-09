import { useEffect, useRef, useState } from "react";
import { EngineService } from "./api.ts";
import { t } from "./i18n.ts";

// The pictures a user message carried (SES-015), loaded from the session's
// attachments once and kept for the rest of the app's run. They are the
// user's own files; nothing a model writes is ever shown as an image.
const pictureCache = new Map<string, string>();

export default function AttachedImages({ sid, images }: { sid: string; images: { name: string; mimeType: string }[] }) {
  const [, setLoaded] = useState(0);
  const [open, setOpen] = useState<string | null>(null);
  // The full-size view takes the focus, so Escape reaches it, and gives it
  // back to the picture it came from.
  const closer = useRef<HTMLButtonElement>(null);
  const opener = useRef<HTMLElement | null>(null);
  useEffect(() => {
    if (open) {
      opener.current = document.activeElement as HTMLElement | null;
      closer.current?.focus();
    }
  }, [open]);
  const close = () => {
    setOpen(null);
    opener.current?.focus();
  };
  useEffect(() => {
    let live = true;
    for (const img of images) {
      const key = `${sid}/${img.name}`;
      if (pictureCache.has(key)) continue;
      EngineService.Attachment(sid, img.name)
        .then((a) => {
          if (!a.mediaType.startsWith("image/")) return;
          pictureCache.set(key, `data:${a.mediaType};base64,${a.data}`);
          if (live) setLoaded((n) => n + 1);
        })
        .catch(() => pictureCache.set(key, ""));
    }
    return () => {
      live = false;
    };
  }, [sid, images]);
  return (
    <div className="msg-pictures">
      {images.map((img, i) => {
        const src = pictureCache.get(`${sid}/${img.name}`);
        if (src === "") return <span key={img.name} className="muted">{t("transcript.picture.missing")}</span>;
        if (!src) return <span key={img.name} className="msg-picture msg-picture-loading" aria-label={t("transcript.picture.loading")} />;
        return (
          <button key={img.name} className="msg-picture" onClick={() => setOpen(src)} aria-label={t("transcript.picture.show", { number: i + 1 })}>
            <img src={src} alt={t("transcript.picture.alt", { number: i + 1 })} />
          </button>
        );
      })}
      {open && (
        <div
          className="picture-view"
          role="dialog"
          aria-modal="true"
          aria-label={t("transcript.picture.dialog")}
          onClick={close}
          onKeyDown={(e) => {
            if (e.key === "Escape") close();
            // Its only control: the focus stays in the dialog.
            if (e.key === "Tab") e.preventDefault();
          }}
        >
          <img src={open} alt={t("transcript.picture.full")} />
          <button ref={closer} className="picture-close" onClick={close}>
            {t("transcript.picture.close")}
          </button>
        </div>
      )}
    </div>
  );
}
