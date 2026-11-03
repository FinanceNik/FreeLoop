// Cost estimates (PRV-008): the price table from config.yaml, then the built-in
// list prices, applied to model calls' tokens. Prices are US dollars per million
// tokens; the first entry whose provider (when set) and model pattern match
// applies, as in config.FindPrice.

export interface PriceEntry {
  providerId: string;
  model: string;
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
  // A write that lasts an hour; without it one costs as cacheWrite.
  cacheWrite1h?: number;
  // The prompt size above which a call pays the row named with LONG_PROMPT.
  longPromptOver?: number;
}

// config.LongPromptSuffix.
export const LONG_PROMPT = " (long prompt)";

export interface ModelCall {
  providerId: string;
  model: string;
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
  // The part of cacheWriteTokens that lasts an hour.
  cacheWrite1hTokens?: number;
  // Adds what earlier calls' counts missed; it is no call itself.
  adjustment?: boolean;
}

// globMatch matches like Go's path.Match: * is any run of characters but
// "/", ? one such character, [...] a class (^ negates). A malformed
// pattern matches nothing.
export function globMatch(pattern: string, s: string): boolean {
  let re = "^";
  for (let i = 0; i < pattern.length; i++) {
    const c = pattern[i];
    if (c === "*") re += "[^/]*";
    else if (c === "?") re += "[^/]";
    else if (c === "\\" && i + 1 < pattern.length) re += escape(pattern[++i]);
    else if (c === "[") {
      const end = pattern.indexOf("]", i + 2);
      if (end < 0) return false;
      const body = pattern.slice(i + 1, end).replace(/\\/g, "\\\\");
      re += "[" + body + "]";
      i = end;
    } else re += escape(c);
  }
  try {
    return new RegExp(re + "$").test(s);
  } catch {
    return false;
  }
}

function escape(c: string): string {
  return c.replace(/[.*+?^${}()|[\]\\/]/g, "\\$&");
}

export function findPrice(prices: readonly PriceEntry[], providerId: string, model: string): PriceEntry | null {
  for (const p of prices) {
    if (p.providerId && p.providerId !== providerId) continue;
    if (p.model === model || globMatch(p.model, model)) return p;
  }
  return null;
}

export interface Estimate {
  cost: number;
  // Calls the table has no price for, which the cost leaves out.
  unpriced: number;
}

// estimate prices calls; a call that does not name its model counts as
// the session's.
export function estimate(prices: readonly PriceEntry[], calls: readonly ModelCall[], session: { providerId: string; model: string }): Estimate {
  let cost = 0;
  let unpriced = 0;
  for (const c of calls) {
    const providerId = c.model ? c.providerId : session.providerId;
    const model = c.model || session.model;
    const base = findPrice(prices, providerId, model);
    if (!base) {
      unpriced++;
      continue;
    }
    const prompt = c.inputTokens + c.cacheReadTokens + c.cacheWriteTokens;
    const long = !c.adjustment && base.longPromptOver && prompt > base.longPromptOver;
    const p = (long && findPrice(prices, providerId, model + LONG_PROMPT)) || base;
    const hour = c.cacheWrite1hTokens ?? 0;
    const write = p.cacheWrite ?? 0;
    cost +=
      (c.inputTokens * p.input +
        c.outputTokens * p.output +
        c.cacheReadTokens * (p.cacheRead ?? 0) +
        (c.cacheWriteTokens - hour) * write +
        hour * (p.cacheWrite1h || write)) /
      1e6;
  }
  return { cost, unpriced };
}

export function formatCost(n: number): string {
  if (n > 0 && n < 0.01) return "<$0.01";
  return `$${n.toFixed(2)}`;
}
