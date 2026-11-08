// A small Markdown parser for agent messages (GUI-003). It produces a tree
// that Markdown.tsx renders as React elements, never as HTML, so text from a
// model cannot inject markup into the app. It covers what coding agents
// write: fenced code, headings, lists, quotes, rules, tables, and inline code,
// bold, italic and links. An unterminated fence renders as code, so partial
// streamed output looks right.
//
// Model output is untrusted, so parsing takes time linear in the text (or
// n log n for code spans) whatever the text: no regular expression here can
// backtrack over more than a constant, inline closers are looked up in
// precomputed tables instead of searched for again from every opener, and
// nesting is bounded. markdown.test.ts holds the hostile inputs.

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

// The s flag lets (.*)$ match a line holding U+2028 or U+2029; without it
// the match failed only after trying every split of the spaces before it,
// quadratic in their number.
// The closing #s are cut by headingText: a lazy group before optional
// spaces and #s backtracks in cubic time on a long line of spaces.
const heading = /^(#{1,6})\s+(.*)$/s;
const bullet = /^\s*[-*+]\s+(.*)$/s;
const numbered = /^\s*(\d{1,9})[.)]\s+(.*)$/s;
const rule = /^\s*([-*_])(\s*\1){2,}\s*$/;
const langName = /^[\w+#.-]*$/;
const sepCell = /^:?-{3,}:?$/;

// fence reads a line opening or closing a fence: ``` or ~~~ and a language.
// It is code rather than one expression because spaces on either side of an
// optional language let an expression split them in quadratically many ways.
function fence(line: string): { marker: string; lang: string } | null {
  const t = line.trim();
  const marker = t.slice(0, 3);
  if (marker !== "```" && marker !== "~~~") return null;
  const lang = t.slice(3).trim();
  return langName.test(lang) ? { marker, lang } : null;
}

// tableSep tells a table's separator row, |---|:---:|, with the outer pipes
// optional; code for the same reason as fence.
function tableSep(line: string): boolean {
  return cells(line).every((c) => sepCell.test(c));
}

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

const lineBreak = /\r\n?|\n/;

export function parseMarkdown(src: string, depth = 0): Block[] {
  return parseLines(src.split(lineBreak), depth).blocks;
}

// parseLines parses lines into blocks. stableLine is the last blank line
// the top-level loop stepped over that is not the last line, and
// stableBlocks the number of blocks before it: every block loop stops at a
// blank line outside a fence and looks at most one line ahead, so those
// blocks are what any text after that line would leave them.
function parseLines(lines: string[], depth: number): { blocks: Block[]; stableLine: number; stableBlocks: number } {
  const blocks: Block[] = [];
  let stableLine = -1;
  let stableBlocks = 0;
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    if (line.trim() === "") {
      if (i < lines.length - 1) {
        stableLine = i;
        stableBlocks = blocks.length;
      }
      i++;
      continue;
    }
    const f = fence(line);
    if (f) {
      const body: string[] = [];
      i++;
      while (i < lines.length && lines[i].trim() !== f.marker) body.push(lines[i++]);
      i++; // closing fence, if any
      blocks.push({ type: "code", lang: f.lang, text: body.join("\n") });
      continue;
    }
    let m: RegExpExecArray | null;
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
      blocks.push({
        type: "quote",
        children: depth < MAX_QUOTE_DEPTH ? parseLines(body, depth + 1).blocks : [{ type: "para", children: parseInline(body.join("\n")) }],
      });
      continue;
    }
    if (line.includes("|") && i + 1 < lines.length && tableSep(lines[i + 1])) {
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
      const items: string[][] = [];
      while (i < lines.length) {
        const l = lines[i];
        const item = ordered ? numbered.exec(l) : bullet.exec(l);
        if (item) {
          items.push([ordered ? item[2] : item[1]]);
        } else if (l.trim() !== "" && /^\s{2,}/.test(l) && items.length > 0) {
          items[items.length - 1].push(l.trim()); // continuation line
        } else {
          break;
        }
        i++;
      }
      blocks.push({ type: "list", ordered, start, items: items.map((parts) => parseInline(parts.join(" "))) });
      continue;
    }
    const para: string[] = [];
    while (
      i < lines.length &&
      lines[i].trim() !== "" &&
      !fence(lines[i]) &&
      !heading.test(lines[i]) &&
      !bullet.test(lines[i]) &&
      !numbered.test(lines[i]) &&
      !lines[i].trimStart().startsWith(">")
    ) {
      para.push(lines[i++]);
    }
    blocks.push({ type: "para", children: parseInline(para.join("\n")) });
  }
  return { blocks, stableLine, stableBlocks };
}

// Parsed is a text's blocks and how much of it later text cannot change:
// stable is where the text after its last finished block starts, and
// stableBlocks how many of blocks come before that.
export type Parsed = { text: string; blocks: Block[]; stable: number; stableBlocks: number };

// parseMarkdownAfter parses text as parseMarkdown does, reusing prev's
// finished blocks when text extends prev's text. A streamed reply grows a
// chunk at a time and is rendered after each, so parsing all of it every
// time would make a long reply quadratic; this parses only from the last
// blank line between blocks, and keeps the earlier blocks' identities so
// their rendering can be skipped too.
export function parseMarkdownAfter(text: string, prev?: Parsed): Parsed {
  if (prev && prev.text === text) return prev;
  // Not startsWith, which V8 compares a character at a time.
  const from = prev && text.length > prev.text.length && text.slice(0, prev.text.length) === prev.text ? prev : undefined;
  const base = from ? from.stable : 0;
  const kept = from ? from.blocks.slice(0, from.stableBlocks) : [];
  // Lines and the breaks between them alternate.
  const parts = text.slice(base).split(/(\r\n?|\n)/);
  const lines: string[] = [];
  for (let k = 0; k < parts.length; k += 2) lines.push(parts[k]);
  const { blocks, stableLine, stableBlocks } = parseLines(lines, 0);
  let stable = base;
  if (stableLine >= 0) for (let k = 0; k <= 2 * stableLine + 1; k++) stable += parts[k].length;
  return { text, blocks: kept.concat(blocks), stable, stableBlocks: kept.length + (stableLine >= 0 ? stableBlocks : 0) };
}

// safeHref accepts only links that open outside the app.
export function safeHref(href: string): string | null {
  return /^https?:\/\//i.test(href.trim()) ? href.trim() : null;
}

const escapable = /[\\`*_[\]()#+\-.!|]/;
const nonWord = /\W/;

export function parseInline(src: string): Inline[] {
  return new InlineParser(src).parse(0, src.length);
}

// InlineParser parses spans of one string. A span's contents are parsed as
// a range of the same string, and each closer is found in a table of where
// the next one is, so a text full of openers that never close costs no
// more than one without them. A construct cannot contain its own kind (its
// contents end at the first closer), so nesting is at most a few levels.
class InlineParser {
  private src: string;
  private next = new Map<string, Int32Array>();
  private runs: Backticks | null = null;
  constructor(src: string) {
    this.src = src;
  }

  // nextOf(key)[i] is the first position at or after i of the character
  // key, or of two of it when key is that character twice; src.length when
  // there is none.
  private nextOf(key: string): Int32Array {
    let t = this.next.get(key);
    if (t) return t;
    const src = this.src;
    const n = src.length;
    const c = key[0];
    const pair = key.length === 2;
    t = new Int32Array(n + 1);
    t[n] = n;
    for (let i = n - 1; i >= 0; i--) t[i] = src[i] === c && (!pair || src[i + 1] === c) ? i : t[i + 1];
    this.next.set(key, t);
    return t;
  }

  // parse parses src[lo:hi) as the old parser parsed src.slice(lo, hi).
  parse(lo: number, hi: number): Inline[] {
    const src = this.src;
    const out: Inline[] = [];
    let text = "";
    const flush = () => {
      if (text) out.push({ type: "text", text });
      text = "";
    };
    let i = lo;
    while (i < hi) {
      const c = src[i];
      const after = i + 1 < hi ? src[i + 1] : undefined;
      if (c === "\\" && after !== undefined && escapable.test(after)) {
        text += after;
        i += 2;
        continue;
      }
      if (c === "`") {
        // A run of r backticks opens a code span closed by the next run at
        // least as long. Without one, its first backtick is text and the
        // rest is tried again as a shorter run, so the span is the longest
        // the runs after it allow, after r minus that many backticks of text.
        let a = i;
        while (a < hi && src[a] === "`") a++;
        const r = a - i;
        const runs = (this.runs ??= new Backticks(src));
        const close = runs.closer(a, hi, r);
        if (!close) {
          text += src.slice(i, a);
          i = a;
          continue;
        }
        text += src.slice(i, a - close.ticks);
        flush();
        out.push({ type: "code", text: src.slice(a, close.at).replace(/^ (.*) $/, "$1") });
        i = close.at + close.ticks;
        continue;
      }
      if ((c === "*" || c === "_") && after === c) {
        const end = this.nextOf(c + c)[i + 2];
        if (end + 1 < hi && end > i + 2) {
          flush();
          out.push({ type: "strong", children: this.parse(i + 2, end) });
          i = end + 2;
          continue;
        }
      }
      if ((c === "*" || c === "_") && after !== " " && after !== c) {
        // _ only at word boundaries, so snake_case stays text.
        const boundary = c === "*" || i === lo || nonWord.test(src[i - 1]);
        const end = this.nextOf(c)[i + 1];
        if (boundary && end < hi && end > i + 1 && src[end - 1] !== " " && (c === "*" || end + 1 >= hi || nonWord.test(src[end + 1]))) {
          flush();
          out.push({ type: "em", children: this.parse(i + 1, end) });
          i = end + 1;
          continue;
        }
      }
      if (c === "[") {
        const close = this.nextOf("]")[i + 1];
        if (close + 1 < hi && src[close + 1] === "(") {
          const end = this.nextOf(")")[close + 2];
          if (end < hi) {
            flush();
            out.push({ type: "link", href: src.slice(close + 2, end), children: this.parse(i + 1, close) });
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
}

// Backticks answers which run of backticks closes a code span, in log time
// over a segment tree of the runs' lengths. A parsed range ends at the end
// of the text or at a * _ or ], so no run crosses a range's end.
class Backticks {
  private starts: number[] = [];
  private size = 1;
  private max: Int32Array;
  constructor(src: string) {
    const lens: number[] = [];
    for (let i = 0; i < src.length; ) {
      if (src[i] !== "`") {
        i++;
        continue;
      }
      let j = i;
      while (j < src.length && src[j] === "`") j++;
      this.starts.push(i);
      lens.push(j - i);
      i = j;
    }
    while (this.size < lens.length) this.size *= 2;
    this.max = new Int32Array(2 * this.size);
    lens.forEach((len, k) => (this.max[this.size + k] = len));
    for (let k = this.size - 1; k > 0; k--) this.max[k] = Math.max(this.max[2 * k], this.max[2 * k + 1]);
  }

  // closer finds, for a run of r backticks ending at a, the longest t <= r
  // for which a run of at least t starts in [a, hi), and the first such run.
  closer(a: number, hi: number, r: number): { at: number; ticks: number } | null {
    const l = this.firstRun(a);
    const h = this.firstRun(hi);
    if (l >= h) return null;
    const ticks = Math.min(r, this.rangeMax(l, h));
    return { at: this.starts[this.firstAtLeast(1, 0, this.size, l, h, ticks)], ticks };
  }

  // firstRun is the index of the first run starting at or after pos.
  private firstRun(pos: number): number {
    let lo = 0;
    let hi = this.starts.length;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      if (this.starts[mid] < pos) lo = mid + 1;
      else hi = mid;
    }
    return lo;
  }

  private rangeMax(l: number, h: number): number {
    let best = 0;
    for (l += this.size, h += this.size; l < h; l >>= 1, h >>= 1) {
      if (l & 1) best = Math.max(best, this.max[l++]);
      if (h & 1) best = Math.max(best, this.max[--h]);
    }
    return best;
  }

  // firstAtLeast is the first run in [l, h) at least t long, searching the
  // node covering runs [nl, nr); -1 if there is none.
  private firstAtLeast(node: number, nl: number, nr: number, l: number, h: number, t: number): number {
    if (nr <= l || nl >= h || this.max[node] < t) return -1;
    if (nr - nl === 1) return nl;
    const mid = (nl + nr) >> 1;
    const left = this.firstAtLeast(2 * node, nl, mid, l, h, t);
    return left >= 0 ? left : this.firstAtLeast(2 * node + 1, mid, nr, l, h, t);
  }
}
