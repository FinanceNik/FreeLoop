import { fit, IMAGE_TYPES, keepAsIs, MAX_BYTES } from "./images.ts";

// prepareImage turns a pasted, dropped or chosen file into what a message
// carries (SES-015): base64 data of an allowed type, within the size
// limits. A picture that is too large is drawn smaller in a canvas; a PNG
// stays a PNG when it fits, anything else becomes a JPEG.

export interface PreparedImage {
  mediaType: string;
  data: string; // base64
  url: string; // an object URL for the thumbnail; revoke when done
}

export async function prepareImage(file: Blob): Promise<PreparedImage> {
  if (!IMAGE_TYPES.includes(file.type)) throw new Error(`${file.type || "This file"} is not a picture type agents read; use PNG, JPEG, GIF or WebP.`);
  const bitmap = await createImageBitmap(file);
  try {
    if (keepAsIs(file.type, file.size, bitmap.width, bitmap.height)) return finish(file, file.type);
    const { w, h } = fit(bitmap.width, bitmap.height);
    const canvas = document.createElement("canvas");
    canvas.width = w;
    canvas.height = h;
    const ctx = canvas.getContext("2d");
    if (!ctx) throw new Error("The picture could not be scaled down.");
    ctx.drawImage(bitmap, 0, 0, w, h);
    if (file.type === "image/png") {
      const png = await blob(canvas, "image/png");
      if (png.size <= MAX_BYTES) return finish(png, "image/png");
    }
    for (const q of [0.85, 0.7, 0.5]) {
      const jpeg = await blob(canvas, "image/jpeg", q);
      if (jpeg.size <= MAX_BYTES) return finish(jpeg, "image/jpeg");
    }
    throw new Error("The picture is too large to send, even scaled down.");
  } finally {
    bitmap.close();
  }
}

function blob(canvas: HTMLCanvasElement, type: string, quality?: number): Promise<Blob> {
  return new Promise((resolve, reject) => canvas.toBlob((b) => (b ? resolve(b) : reject(new Error("The picture could not be encoded."))), type, quality));
}

async function finish(b: Blob, mediaType: string): Promise<PreparedImage> {
  const bytes = new Uint8Array(await b.arrayBuffer());
  let bin = "";
  for (let i = 0; i < bytes.length; i += 0x8000) bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return { mediaType, data: btoa(bin), url: URL.createObjectURL(b) };
}
