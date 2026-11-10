// The cost summary to share (GUI-029): what the usage page's range and filters cost,
// by agent and by model, at list prices. Pure, so costsummary.test.ts covers it. It
// names no session or project, so it can go in a public comment as it is, and nothing
// is sent anywhere: the user copies or saves it. It is written in the language in force
// when it is made (GUI-048).

import { compact, groupBy, tokensOf, type Group, type Row } from "./analytics.ts";
import { formatCost } from "./cost.ts";
import { list, t } from "./i18n.ts";

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
  g.cost === 0 && g.unpricedTokens >= tokensOf(g) ? t("costSummary.noPrice") : formatCost(g.cost);

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
  return [...shown, line(t("costSummary.more", { count: rest.length }), sum)];
}

export function costSummary<R extends Row>(
  rows: R[],
  keys: { agent: (r: R) => string; model: (r: R) => string },
  range: { label: string; since: string; until: string; pricesAsOf: string; filtered: boolean; cacheReads: boolean },
  // What else the summary says: how agents reported (reporting.ts) and what the
  // policy held.
  more: string[] = [],
): CostSummary {
  const unpriced = rows.filter((r) => !r.priced);
  const whole = groupBy(rows, () => "all", "tokens")[0];
  const notes = [t("costSummary.listPrices", { date: range.pricesAsOf })];
  if (unpriced.length > 0) {
    const names = [...new Set(unpriced.map(keys.model))].sort();
    const calls = unpriced.reduce((n, r) => n + r.calls, 0);
    notes.push(t("costSummary.unpriced", { count: calls, models: list(names) }));
  }
  notes.push(...more.filter(Boolean));
  if (!range.cacheReads) notes.push(t("costSummary.noCacheReads"));
  if (range.filtered) notes.push(t("costSummary.filtered"));
  return {
    title: t("costSummary.heading", { range: range.label }),
    dates: t("costSummary.dates", { since: range.since, until: range.until }),
    agents: lines(rows, keys.agent),
    models: lines(rows, keys.model),
    total: { name: t("costSummary.total"), tokens: compact(whole ? tokensOf(whole) : 0), cost: whole ? costText(whole) : t("costSummary.noPrice") },
    notes,
  };
}

// A name can hold a | or a line break, which would break the table it goes in.
const cell = (s: string) => s.replace(/\|/g, "\\|").replace(/\s+/g, " ").trim();

function table(heading: string, rows: SummaryLine[]): string[] {
  return [`| ${heading} | ${t("costSummary.tokens")} | ${t("costSummary.cost")} |`, "|:---|---:|---:|", ...rows.map((r) => `| ${cell(r.name)} | ${r.tokens} | ${r.cost} |`)];
}

// summaryText is Markdown that renders as tables in a GitHub comment, its headings in
// the language in force.
export function summaryText(s: CostSummary): string {
  return [
    `**${s.title}** (${s.dates})`,
    "",
    ...table(t("costSummary.agent"), s.agents),
    "",
    ...table(t("costSummary.model"), [...s.models, { ...s.total, name: `**${s.total.name}**`, tokens: `**${s.total.tokens}**`, cost: `**${s.total.cost}**` }]),
    "",
    ...s.notes,
  ].join("\n");
}

// The picture's typeface, and the weight and size of its notes.
const FAMILY = "-apple-system,Helvetica,Arial,sans-serif";
const NOTE = "400 14px";

// drawSummary draws the picture: a fixed light card, whatever the app's theme, so it
// reads the same wherever it is pasted. Text goes in a size that survives a chat's
// scaling, and long names are cut rather than run into the numbers.
export function drawSummary(s: CostSummary, ctx: CanvasRenderingContext2D, scale = 2): { width: number; height: number } {
  const width = 720;
  const pad = 32;
  const rowH = 30;
  const noteLines = (text: string) => wrap(ctx, text, width - pad * 2, `${NOTE} ${FAMILY}`);
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
  const text = (words: string, x: number, y: number, font: string, color: string, align: CanvasTextAlign = "left") => {
    ctx.font = `${font} ${FAMILY}`;
    ctx.fillStyle = color;
    ctx.textAlign = align;
    ctx.fillText(words, x, y);
  };
  const costX = width - pad;
  const tokensX = costX - 150;
  const nameMax = tokensX - 90 - pad;
  let y = pad + 22;
  text(s.title, pad, y, "700 24px", ink);
  y += 28;
  text(s.dates, pad, y, "400 15px", muted);
  y += 20;
  const section = (heading: string, entries: SummaryLine[], total?: SummaryLine) => {
    y += rowH;
    text(heading, pad, y, "600 14px", muted);
    text(t("costSummary.tokens"), tokensX, y, "600 14px", muted, "right");
    text(t("costSummary.cost"), costX, y, "600 14px", muted, "right");
    ctx.strokeStyle = "#cfd2d9";
    ctx.beginPath();
    ctx.moveTo(pad, y + 8);
    ctx.lineTo(width - pad, y + 8);
    ctx.stroke();
    for (const r of total ? [...entries, total] : entries) {
      y += rowH;
      const weight = r === total ? "700 16px" : "400 16px";
      ctx.font = `${weight} ${FAMILY}`;
      text(fit(ctx, r.name, nameMax), pad, y, weight, ink);
      text(r.tokens, tokensX, y, weight, ink, "right");
      text(r.cost, costX, y, weight, ink, "right");
    }
  };
  section(t("costSummary.agent"), s.agents);
  section(t("costSummary.model"), s.models, s.total);
  y += 16;
  for (const line of notes) {
    y += 20;
    text(line, pad, y, "400 14px", muted);
  }
  return { width, height };
}

function fit(ctx: CanvasRenderingContext2D, name: string, max: number): string {
  if (ctx.measureText(name).width <= max) return name;
  let cut = name;
  while (cut.length > 1 && ctx.measureText(`${cut}…`).width > max) cut = cut.slice(0, -1);
  return `${cut}…`;
}

function wrap(ctx: CanvasRenderingContext2D, words: string, max: number, font: string): string[] {
  ctx.font = font;
  const out: string[] = [];
  let line = "";
  for (const word of words.split(" ")) {
    const next = line ? `${line} ${word}` : word;
    if (line && ctx.measureText(next).width > max) {
      out.push(line);
      line = word;
    } else line = next;
  }
  if (line) out.push(line);
  return out;
}

