// Cost estimates (PRV-008): the price table from config.yaml, then the built-in
// list prices, applied to model calls' tokens. Prices are US dollars per million
// tokens; the first entry whose provider (when set) and model pattern match
// applies, as in config.FindPrice.
import { numberFormat } from "./i18n.ts";

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
  const whole = matchPrice(prices, providerId, model);
  if (whole) return whole;
  // An agent may name a model with its provider, openai/gpt-5.5, or with its effort,
  // gpt-5.5/high, as config.FindPrice allows (PRV-008).
  // Only for an agent's usage, which names no provider (config.FindPrice does the same).
  const slash = model.indexOf("/");
  if (slash < 0 || providerId) return null;
  return matchPrice(prices, providerId, model.slice(slash + 1)) ?? matchPrice(prices, providerId, model.slice(0, slash));
}

function matchPrice(prices: readonly PriceEntry[], providerId: string, model: string): PriceEntry | null {
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

// formatCost writes an amount in US dollars, as provider prices are, in the locale's
// style: $146.51, 146,51 $, $ 146,51 (GUI-048).
export function formatCost(n: number): string {
  const dollars = numberFormat({ style: "currency", currency: "USD", currencyDisplay: "narrowSymbol" });
  if (n > 0 && n < 0.01) return `<${dollars.format(0.01)}`;
  return dollars.format(n);
}

// costText is a session's estimated cost in words (PRV-008): what the price table
// makes of its calls, and how the agent counted them, so a missing count never reads
// as nothing spent.
export function costText(e: Estimate, calls: number, measured: { turns: number; unreported: number }): string {
  if (calls === 0) return measured.unreported > 0 ? "not reported by the agent" : "none yet";
  let text = e.unpriced === calls ? "no price set" : `≈ ${formatCost(e.cost)}${e.unpriced ? "+" : ""}`;
  if (measured.turns > 0) text += ", from the agent's turn totals";
  if (measured.unreported > 0) text += `; ${measured.unreported === 1 ? "1 turn" : `${measured.unreported} turns`} not reported`;
  return text;
}
