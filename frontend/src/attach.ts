import { IMAGE_TYPES } from "./images.ts";

// Files attached to a message from anywhere on the Mac (SES-019). A picture goes
// to the model as a picture and text goes in the message. Anything else cannot
// travel in a message, so it says so instead of sending bytes the model cannot read.
// The limits match those of the engine's other attachments.

export const MAX_FILES = 20;
export const MAX_FILE_BYTES = 256 << 10;
export const MAX_FILES_TOTAL = 1 << 20;

export function isPicture(type: string): boolean {
  return IMAGE_TYPES.includes(type);
}

// textOf returns a file's bytes as text, or null when they are not text.
export function textOf(bytes: Uint8Array): string | null {
  if (bytes.includes(0)) return null;
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    return null;
  }
}

export interface TextFile {
  name: string;
  text: string;
}

// addText adds a file to those already attached, or says why it cannot be.
export function addText(list: TextFile[], name: string, bytes: Uint8Array): { list: TextFile[]; error?: string } {
  if (bytes.length > MAX_FILE_BYTES) return { list, error: `${name} is larger than 256 KB. Put it in the project and ask for the part you need.` };
  const text = textOf(bytes);
  if (text === null) return { list, error: `${name} is not text or a picture, so it cannot go in a message. Put it in the project and name its path.` };
  if (list.length >= MAX_FILES) return { list, error: `A message takes at most ${MAX_FILES} files.` };
  if (list.reduce((n, f) => n + f.text.length, 0) + text.length > MAX_FILES_TOTAL) return { list, error: `${name} would take this message over its 1 MB limit.` };
  return { list: [...list.filter((f) => f.name !== name), { name, text }] };
}
