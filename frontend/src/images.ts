// Pictures in prompts (SES-015): the limits the page holds them to before
// they reach the engine. Pure, so images.test.ts covers it; the canvas work
// is in imageprep.ts.

export const MAX_IMAGES = 10;
// The longer side and size a picture is scaled down to: within every
// provider's limit, and small enough to send on every turn.
export const MAX_SIDE = 2000;
export const MAX_BYTES = 3_750_000;
export const IMAGE_TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp"];

// fit returns the size a picture of w×h is drawn at: unchanged when it
// fits, else scaled to max on its longer side, keeping its shape.
export function fit(w: number, h: number, max = MAX_SIDE): { w: number; h: number } {
  if (w <= max && h <= max) return { w, h };
  const s = max / Math.max(w, h);
  return { w: Math.max(1, Math.round(w * s)), h: Math.max(1, Math.round(h * s)) };
}

// keepAsIs reports whether a picture can go unchanged.
export function keepAsIs(type: string, bytes: number, w: number, h: number): boolean {
  return IMAGE_TYPES.includes(type) && bytes <= MAX_BYTES && w <= MAX_SIDE && h <= MAX_SIDE;
}

// room returns how many of n new pictures fit beside have already attached.
export function room(have: number, n: number): number {
  return Math.max(0, Math.min(n, MAX_IMAGES - have));
}
