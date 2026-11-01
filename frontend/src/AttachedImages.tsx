import { useEffect, useState } from "react";
import { EngineService } from "./api.ts";

// The pictures a user message carried (SES-015), loaded from the session's
// attachments once and kept for the rest of the app's run. They are the
// user's own files; nothing a model writes is ever shown as an image.
const pictureCache = new Map<string, string>();

export default function AttachedImages({ sid, images }: { sid: string; images: { name: string; mimeType: string }[] }) {
  const [, setLoaded] = useState(0);
  const [open, setOpen] = useState<string | null>(null);
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
        if (src === "") return <span key={img.name} className="muted">[picture missing]</span>;
        if (!src) return <span key={img.name} className="msg-picture msg-picture-loading" aria-label="Loading picture" />;
        return (
          <button key={img.name} className="msg-picture" onClick={() => setOpen(src)} aria-label={`Show picture ${i + 1} full size`}>
            <img src={src} alt={`Picture ${i + 1}`} />
          </button>
        );
      })}
      {open && (
        <div className="picture-view" role="dialog" aria-label="Picture" onClick={() => setOpen(null)} onKeyDown={(e) => e.key === "Escape" && setOpen(null)}>
          <img src={open} alt="Attached picture, full size" />
        </div>
      )}
    </div>
  );
}
