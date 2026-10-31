// A small Markdown parser for agent messages (GUI-003). It produces a tree
// that Markdown.tsx renders as React elements, never as HTML, so text from a
// model cannot inject markup into the app. It covers what coding agents
// write: fenced code, headings, lists, quotes, rules, tables, and inline code,
// bold, italic and links. An unterminated fence renders as code, so partial
// streamed output looks right.

export type Inline =
  | { type: "text"; text: string }
  | { type: "code"; text: string }
  | { type: "strong"; children: Inline[] }
  | { type: "em"; children: Inline[] }
  | { type: "link"; href: string; children: Inline[] };

export type Block =
  | { type: "code"; lang: string; text: string }
  | { type: "heading"; level: number; children: Inline[] }
  | { type: "list"; ordered: boolean; start: number; items: Inline[][] }
  | { type: "quote"; children: Block[] }
  | { type: "hr" }
  | { type: "table"; header: Inline[][]; rows: Inline[][][] }
  | { type: "para"; children: Inline[] };

const fence = /^\s*(```|~~~)\s*([\w+#.-]*)\s*$/;
// The closing #s are cut by headingText: a lazy group before optional
// spaces and #s backtracks in cubic time on a long line of spaces.
const heading = /^(#{1,6})\s+(.*)$/;
const bullet = /^\s*[-*+]\s+(.*)$/;
const numbered = /^\s*(\d{1,9})[.)]\s+(.*)$/;
const rule = /^\s*([-*_])(\s*\1){2,}\s*$/;
const tableSep = /^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?\s*$/;

// headingText drops a heading's closing #s, which need a space before them.
function headingText(s: string): string {
  let t = s.trimEnd();
  let j = t.length;
  while (j > 0 && t[j - 1] === "#") j--;
  if (j < t.length && (j === 0 || t[j - 1] === " " || t[j - 1] === "\t")) t = t.slice(0, j).trimEnd();
  return t;
}

// Quotes nest deeper than this only as text: each level is a recursive
// parse, and thousands of > would overflow the stack.
const MAX_QUOTE_DEPTH = 16;

function cells(line: string): string[] {
  let s = line.trim();
  if (s.startsWith("|")) s = s.slice(1);
  if (s.endsWith("|")) s = s.slice(0, -1);
  return s.split("|").map((c) => c.trim());
}

export function parseMarkdown(src: string, depth = 0): Block[] {
  const lines = src.replace(/\r\n?/g, "\n").split("\n");
  const blocks: Block[] = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.trim() === "") {
      i++;
      continue;
    }
    let m = fence.exec(line);
    if (m) {
      const marker = m[1];
      const body: string[] = [];
      i++;
      while (i < lines.length && lines[i].trim() !== marker) body.push(lines[i++]);
      i++; // closing fence, if any
      blocks.push({ type: "code", lang: m[2], text: body.join("\n") });
      continue;
    }
    if ((m = heading.exec(line))) {
      blocks.push({ type: "heading", level: m[1].length, children: parseInline(headingText(m[2])) });
      i++;
      continue;
    }
    if (rule.test(line)) {
      blocks.push({ type: "hr" });
      i++;
      continue;
    }
    if (line.trimStart().startsWith(">")) {
      const body: string[] = [];
      while (i < lines.length && lines[i].trimStart().startsWith(">")) body.push(lines[i++].trimStart().replace(/^>\s?/, ""));
      const inner = body.join("\n");
      blocks.push({ type: "quote", children: depth < MAX_QUOTE_DEPTH ? parseMarkdown(inner, depth + 1) : [{ type: "para", children: parseInline(inner) }] });
      continue;
    }
    if (line.includes("|") && i + 1 < lines.length && tableSep.test(lines[i + 1])) {
      const header = cells(line).map(parseInline);
      i += 2;
      const rows: Inline[][][] = [];
      while (i < lines.length && lines[i].includes("|") && lines[i].trim() !== "") rows.push(cells(lines[i++]).map(parseInline));
      blocks.push({ type: "table", header, rows });
      continue;
    }
    const b = bullet.exec(line);
    const n = numbered.exec(line);
    if (b || n) {
      const ordered = !b;
      const start = n ? Number(n[1]) : 1;
      const items: string[] = [];
      while (i < lines.length) {
        const l = lines[i];
        const item = ordered ? numbered.exec(l) : bullet.exec(l);
        if (item) {
          items.push(ordered ? item[2] : item[1]);
        } else if (l.trim() !== "" && /^\s{2,}/.test(l) && items.length > 0) {
          items[items.length - 1] += " " + l.trim(); // continuation line
        } else {
          break;
        }
        i++;
      }
      blocks.push({ type: "list", ordered, start, items: items.map(parseInline) });
      continue;
    }
    const para: string[] = [];
    while (
      i < lines.length &&
      lines[i].trim() !== "" &&
      !fence.test(lines[i]) &&
      !heading.test(lines[i]) &&
      !bullet.test(lines[i]) &&
      !numbered.test(lines[i]) &&
      !lines[i].trimStart().startsWith(">")
    ) {
      para.push(lines[i++]);
    }
    blocks.push({ type: "para", children: parseInline(para.join("\n")) });
  }
  return blocks;
}

// safeHref accepts only links that open outside the app.
export function safeHref(href: string): string | null {
  return /^https?:\/\//i.test(href.trim()) ? href.trim() : null;
}

export function parseInline(src: string): Inline[] {
  const out: Inline[] = [];
  let text = "";
  const flush = () => {
    if (text) out.push({ type: "text", text });
    text = "";
  };
  let i = 0;
  while (i < src.length) {
    const c = src[i];
    if (c === "\\" && i + 1 < src.length && /[\\`*_[\]()#+\-.!|]/.test(src[i + 1])) {
      text += src[i + 1];
      i += 2;
      continue;
    }
    if (c === "`") {
      let ticks = 1;
      while (src[i + ticks] === "`") ticks++;
      const marker = "`".repeat(ticks);
      const end = src.indexOf(marker, i + ticks);
      if (end > 0) {
        flush();
        out.push({ type: "code", text: src.slice(i + ticks, end).replace(/^ (.*) $/, "$1") });
        i = end + ticks;
        continue;
      }
    }
    if ((c === "*" || c === "_") && src[i + 1] === c) {
      const end = src.indexOf(c + c, i + 2);
      if (end > i + 2) {
        flush();
        out.push({ type: "strong", children: parseInline(src.slice(i + 2, end)) });
        i = end + 2;
        continue;
      }
    }
    if ((c === "*" || c === "_") && src[i + 1] !== " " && src[i + 1] !== c) {
      // _ only at word boundaries, so snake_case stays text.
      const boundary = c === "*" || i === 0 || /\W/.test(src[i - 1]);
      const end = src.indexOf(c, i + 1);
      if (boundary && end > i + 1 && src[end - 1] !== " " && (c === "*" || end + 1 >= src.length || /\W/.test(src[end + 1]))) {
        flush();
        out.push({ type: "em", children: parseInline(src.slice(i + 1, end)) });
        i = end + 1;
        continue;
      }
    }
    if (c === "[") {
      const close = src.indexOf("]", i + 1);
      if (close > i && src[close + 1] === "(") {
        const end = src.indexOf(")", close + 2);
        if (end > close) {
          flush();
          out.push({ type: "link", href: src.slice(close + 2, end), children: parseInline(src.slice(i + 1, close)) });
          i = end + 1;
          continue;
        }
      }
    }
    text += c;
    i++;
  }
  flush();
  return out;
}
