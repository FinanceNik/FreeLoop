import { useEffect, useRef, useState } from "react";
import { EngineService, errorText } from "./api.ts";
import { PDF_FRAME_ID, pdfBytes, viewerDocument } from "./pdfview.ts";

// PdfView shows a PDF of the session's folder (GUI-035), in the frames pdfview.ts explains.
export default function PdfView({ sessionId, path }: { sessionId: string; path: string }) {
  const [problem, setProblem] = useState<string | null>(null);
  const [url, setUrl] = useState<string | null>(null);
  const viewer = useRef<HTMLIFrameElement>(null);

  useEffect(() => {
    let live = true;
    let made: string | null = null;
    setProblem(null);
    setUrl(null);
    EngineService.ViewPDF(sessionId, path)
      .then((pdf) => {
        if (!live) return;
        made = URL.createObjectURL(new Blob([pdfBytes(pdf?.data ?? "")], { type: "application/pdf" }));
        setUrl(made);
      })
      .catch((err) => live && setProblem(errorText(err)));
    return () => {
      live = false;
      if (made) URL.revokeObjectURL(made);
    };
  }, [sessionId, path]);

  const place = () => {
    const inner = viewer.current?.contentDocument?.getElementById(PDF_FRAME_ID) as HTMLIFrameElement | null;
    if (inner && url) inner.src = url;
  };
  useEffect(place, [url]);

  if (problem) return <p className="muted">{problem}</p>;
  // Keyed by the file: a frame that has shown one PDF starts clean for the next.
  return <iframe key={path} ref={viewer} className="file-pdf" title={`PDF: ${path}`} srcDoc={viewerDocument()} onLoad={place} />;
}
