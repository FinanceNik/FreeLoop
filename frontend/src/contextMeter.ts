// How full a session's context is, in the few characters a pane's header has room for
// (GUI-036). All from usage events the app already has: this makes no model call.

export interface ContextSize {
  tokens: number;
  // The model's window; 0 when nothing has said what it is.
  window: number;
}

// From here a session is worth acting on. The built-in loop compacts by itself at this
// share too (HRN-013).
const HIGH_SHARE = 0.8;

// A resend below this size costs too little to mention.
const RESEND_MIN_TOKENS = 50_000;
// A resend wrote at least this share of the context to the cache...
const RESEND_MIN_WRITTEN = 0.5;
// ...and the context was already about this size, within this factor. If it was much
// smaller, the context grew by what was just added; if much larger, it was compacted.
const RESEND_SAME_SIZE = 1.25;

export function shortTokens(tokens: number): string {
  if (tokens >= 999_500) return `${(tokens / 1_000_000).toFixed(1)}M`;
  if (tokens >= 1000) return `${Math.round(tokens / 1000)}k`;
  return String(tokens);
}

// contextLabel is the meter's text, or null when there is no figure: never a zero.
export function contextLabel(c: ContextSize | null | undefined): string | null {
  if (!c || c.tokens <= 0) return null;
  if (c.window <= 0) return shortTokens(c.tokens);
  const percent = (100 * c.tokens) / c.window;
  return percent < 1 ? "<1 %" : `${Math.round(percent)} %`;
}

// contextHigh needs a window: a size alone does not say how close the limit is.
export function contextHigh(c: ContextSize | null | undefined): boolean {
  return !!c && c.window > 0 && c.tokens >= HIGH_SHARE * c.window;
}

export function contextTitle(c: ContextSize): string {
  const size = c.window > 0 ? `${c.tokens.toLocaleString()} of ${c.window.toLocaleString()} tokens` : `${c.tokens.toLocaleString()} tokens`;
  return `Context: ${size}. Every message sends this again, so a fuller context uses your limits faster.`;
}

// contextResent says a call wrote nearly all of a large context to the cache although
// the context had not changed much: the cache no longer held it.
export function contextResent(before: number, context: number, written: number): boolean {
  return context >= RESEND_MIN_TOKENS && written >= RESEND_MIN_WRITTEN * context && before * RESEND_SAME_SIZE >= context && before <= RESEND_SAME_SIZE * context;
}
