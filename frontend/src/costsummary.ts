// The cost summary to share (GUI-029): what the usage page's range and filters cost,
// by agent and by model, at list prices. Pure, so costsummary.test.ts covers it. It
// names no session or project, so it can go in a public comment as it is, and nothing
// is sent anywhere: the user copies or saves it.

import { compact, groupBy, tokensOf, type Group, type Row } from "./analytics.ts";
import { formatCost } from "./cost.ts";

export interface SummaryLine {
  name: string;
  tokens: string;
  cost: string;
}

export interface CostSummary {
  title: string;
  dates: string;
  agents: SummaryLine[];
  models: SummaryLine[];
  total: SummaryLine;
  notes: string[];
}

// A chat picture and a comment stay readable; the rest fold together.
const MOST_LINES = 8;

const costText = (g: Pick<Group, "input" | "output" | "cacheRead" | "cacheWrite" | "cost" | "unpricedTokens">) =>
  // Nothing priced says so, rather than claiming that nothing was spent.
  g.cost === 0 && g.unpricedTokens >= tokensOf(g) ? "no price" : formatCost(g.cost);

function lines<R extends Row>(rows: R[], key: (r: R) => string): SummaryLine[] {
  const groups: Group[] = groupBy(rows, key, "tokens");
  const line = (name: string, g: Group): SummaryLine => ({ name, tokens: compact(tokensOf(g)), cost: costText(g) });
  const shown = groups.slice(0, MOST_LINES).map((g) => line(g.key, g));
  const rest = groups.slice(MOST_LINES);
  if (rest.length === 0) return shown;
  const sum = rest.reduce((a, g) => ({
    ...a,
    input: a.input + g.input,
    output: a.output + g.output,
    cacheRead: a.cacheRead + g.cacheRead,
    cacheWrite: a.cacheWrite + g.cacheWrite,
    cost: a.cost + g.cost,
    unpricedTokens: a.unpricedTokens + g.unpricedTokens,
  }));
  return [...shown, line(`${rest.length} more`, sum)];
}

export function costSummary<R extends Row>(
  rows: R[],
  keys: { agent: (r: R) => string; model: (r: R) => string },
  range: { label: string; since: string; until: string; pricesAsOf: string; filtered: boolean; cacheReads: boolean },
): CostSummary {
  const unpriced = rows.filter((r) => !r.priced);
  const whole = groupBy(rows, () => "all", "tokens")[0];
  const notes = [`Estimated at list prices as of ${range.pricesAsOf}. An agent on a subscription is shown at what the same work costs on the API, not at what the subscription costs.`];
  if (unpriced.length > 0) {
    const names = [...new Set(unpriced.map(keys.model))].sort();
    const calls = unpriced.reduce((n, r) => n + r.calls, 0);
    notes.push(`${calls} model ${calls === 1 ? "call" : "calls"} on ${names.join(", ")} ${calls === 1 ? "has" : "have"} no price, so the cost leaves ${calls === 1 ? "it" : "them"} out.`);
  }
  if (!range.cacheReads) notes.push("Tokens leave out cache reads.");
  if (range.filtered) notes.push("A filtered part of the usage, not all of it.");
  return {
    title: `Freeloop usage, last ${range.label}`,
    dates: `${range.since} to ${range.until}`,
    agents: lines(rows, keys.agent),
    models: lines(rows, keys.model),
    total: { name: "Total", tokens: compact(whole ? tokensOf(whole) : 0), cost: whole ? costText(whole) : "no price" },
    notes,
  };
}

// A name can hold a | or a line break, which would break the table it goes in.
const cell = (s: string) => s.replace(/\|/g, "\\|").replace(/\s+/g, " ").trim();

function table(heading: string, rows: SummaryLine[]): string[] {
  return [`| ${heading} | Tokens | Estimated cost |`, "|:---|---:|---:|", ...rows.map((r) => `| ${cell(r.name)} | ${r.tokens} | ${r.cost} |`)];
}

// summaryText is Markdown that renders as tables in a GitHub comment.
export function summaryText(s: CostSummary): string {
  return [
    `**${s.title}** (${s.dates})`,
    "",
    ...table("Agent", s.agents),
    "",
    ...table("Model", [...s.models, { ...s.total, name: `**${s.total.name}**`, tokens: `**${s.total.tokens}**`, cost: `**${s.total.cost}**` }]),
    "",
    ...s.notes,
  ].join("\n");
}

// drawSummary draws the picture: a fixed light card, whatever the app's theme, so it
// reads the same wherever it is pasted. Text goes in a size that survives a chat's
// scaling, and long names are cut rather than run into the numbers.
export function drawSummary(s: CostSummary, ctx: CanvasRenderingContext2D, scale = 2): { width: number; height: number } {
  const width = 720;
  const pad = 32;
  const rowH = 30;
  const noteLines = (text: string) => wrap(ctx, text, width - pad * 2, "400 14px -apple-system, Helvetica, Arial, sans-serif");
  const notes = s.notes.flatMap(noteLines);
  const rows = s.agents.length + s.models.length + 3;
  const height = pad + 70 + rows * rowH + 34 + notes.length * 20 + pad;
  ctx.canvas.width = width * scale;
  ctx.canvas.height = height * scale;
  ctx.scale(scale, scale);
  ctx.fillStyle = "#ffffff";
  ctx.fillRect(0, 0, width, height);
  const ink = "#1d1f23";
  const muted = "#5f636b";
  const text = (t: string, x: number, y: number, font: string, color: string, align: CanvasTextAlign = "left") => {
    ctx.font = `${font} -apple-system, Helvetica, Arial, sans-serif`;
    ctx.fillStyle = color;
    ctx.textAlign = align;
    ctx.fillText(t, x, y);
  };
  const costX = width - pad;
  const tokensX = costX - 150;
  const nameMax = tokensX - 90 - pad;
  let y = pad + 22;
  text(s.title, pad, y, "700 24px", ink);
  y += 28;
  text(s.dates, pad, y, "400 15px", muted);
  y += 20;
  const section = (heading: string, list: SummaryLine[], total?: SummaryLine) => {
    y += rowH;
    text(heading, pad, y, "600 14px", muted);
    text("Tokens", tokensX, y, "600 14px", muted, "right");
    text("Estimated cost", costX, y, "600 14px", muted, "right");
    ctx.strokeStyle = "#cfd2d9";
    ctx.beginPath();
    ctx.moveTo(pad, y + 8);
    ctx.lineTo(width - pad, y + 8);
    ctx.stroke();
    for (const r of total ? [...list, total] : list) {
      y += rowH;
      const weight = r === total ? "700 16px" : "400 16px";
      ctx.font = `${weight} -apple-system, Helvetica, Arial, sans-serif`;
      text(fit(ctx, r.name, nameMax), pad, y, weight, ink);
      text(r.tokens, tokensX, y, weight, ink, "right");
      text(r.cost, costX, y, weight, ink, "right");
    }
  };
  section("Agent", s.agents);
  section("Model", s.models, s.total);
  y += 16;
  for (const line of notes) {
    y += 20;
    text(line, pad, y, "400 14px", muted);
  }
  return { width, height };
}

function fit(ctx: CanvasRenderingContext2D, t: string, max: number): string {
  if (ctx.measureText(t).width <= max) return t;
  let cut = t;
  while (cut.length > 1 && ctx.measureText(`${cut}…`).width > max) cut = cut.slice(0, -1);
  return `${cut}…`;
}

function wrap(ctx: CanvasRenderingContext2D, t: string, max: number, font: string): string[] {
  ctx.font = font;
  const out: string[] = [];
  let line = "";
  for (const word of t.split(" ")) {
    const next = line ? `${line} ${word}` : word;
    if (line && ctx.measureText(next).width > max) {
      out.push(line);
      line = word;
    } else line = next;
  }
  if (line) out.push(line);
  return out;
}

