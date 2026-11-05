// Showing a PDF inside the app (GUI-035). WebKit draws one with its own viewer, but only in
// a frame without a sandbox, where one click on a link in the file used to take that frame
// or the whole page elsewhere (OQ-035). So the PDF's frame sits inside a viewer frame whose
// policy lets it load nothing but the PDF: WebKit then refuses the navigation itself. Pure,
// so pdfview.test.ts holds it to that.

// VIEWER_POLICY allows the blob and nothing else: no scripts, no other frame, no fetching.
export const VIEWER_POLICY = "default-src 'none'; frame-src blob:; style-src 'unsafe-inline'";

export const PDF_FRAME_ID = "pdf";

// viewerDocument is the viewer frame's page. The app puts the blob in its inner frame, so
// it needs no script of its own.
export function viewerDocument(): string {
  return `<!doctype html><meta http-equiv="Content-Security-Policy" content="${VIEWER_POLICY}"><style>html,body{height:100%;margin:0}iframe{display:block;width:100%;height:100%;border:0}</style><iframe id="${PDF_FRAME_ID}" title="PDF"></iframe>`;
}

// pdfBytes decodes what the engine sends.
export function pdfBytes(base64: string): Uint8Array<ArrayBuffer> {
  const text = atob(base64);
  const out = new Uint8Array(text.length);
  for (let i = 0; i < text.length; i++) out[i] = text.charCodeAt(i);
  return out;
}
