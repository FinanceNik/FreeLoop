import { Component, memo, useMemo, useRef, useState, type ReactNode } from "react";
import { Browser, Clipboard } from "@wailsio/runtime";
import { parseMarkdownAfter, safeHref, type Block, type Inline, type Parsed } from "./markdown.ts";
import { locale, t } from "./i18n.ts";

// Markdown renders agent text as React elements (see markdown.ts): no HTML
// from the model ever reaches the DOM. The text is shown as given; the code
// block's own words (Copy) follow the language, so the memos below re-render
// on a change of language as well as of the text (GUI-048).
export function Markdown({ text }: { text: string }) {
  return <Shown text={text} locale={locale()} />;
}

const Shown = memo(function Shown({ text, locale }: { text: string; locale: string }) {
  return (
    <Plain text={text}>
      <Rendered text={text} locale={locale} />
    </Plain>
  );
});

// Rendered parses a streamed reply only from its last finished block on
// (parseMarkdownAfter), and a finished block keeps its identity, so
// BlockView skips rendering it again as the reply grows.
function Rendered({ text, locale }: { text: string; locale: string }) {
  const last = useRef<Parsed | undefined>(undefined);
  const { blocks } = useMemo(() => (last.current = parseMarkdownAfter(text, last.current)), [text]);
  return (
    <div className="md">
      {blocks.map((b, i) => (
        <BlockView key={i} block={b} locale={locale} />
      ))}
    </div>
  );
}

// locale is not read here: a change of it renders the block again, and with it
// the code block's words.
const BlockView = memo(function BlockView({ block }: { block: Block; locale: string }) {
  return renderBlock(block, 0);
});

// Plain shows the text as it is if rendering it fails, so one message the
// parser cannot handle never takes down the window, which would come back
// with the session on every launch. It tries again when the text changes.
class Plain extends Component<{ text: string; children: ReactNode }, { failed: string | null }> {
  state = { failed: null as string | null };
  static getDerivedStateFromError() {
    return { failed: "" };
  }
  componentDidCatch(err: unknown) {
    console.error("markdown: rendering failed", err);
    this.setState({ failed: this.props.text });
  }
  componentDidUpdate(prev: { text: string }) {
    if (this.state.failed !== null && prev.text !== this.props.text) this.setState({ failed: null });
  }
  render() {
    if (this.state.failed !== null) return <pre className="md-plain">{this.props.text}</pre>;
    return this.props.children;
  }
}

function renderBlock(b: Block, key: number): ReactNode {
  switch (b.type) {
    case "code":
      return <CodeBlock key={key} lang={b.lang} text={b.text} />;
    case "heading": {
      const H = `h${Math.min(b.level + 2, 6)}` as "h3" | "h4" | "h5" | "h6";
      return <H key={key}>{renderInline(b.children)}</H>;
    }
    case "list": {
      const items = b.items.map((item, i) => <li key={i}>{renderInline(item)}</li>);
      return b.ordered ? (
        <ol key={key} start={b.start}>
          {items}
        </ol>
      ) : (
        <ul key={key}>{items}</ul>
      );
    }
    case "quote":
      return <blockquote key={key}>{b.children.map((c, i) => renderBlock(c, i))}</blockquote>;
    case "hr":
      return <hr key={key} />;
    case "table":
      return (
        <div key={key} className="md-table">
          <table>
            <thead>
              <tr>
                {b.header.map((c, i) => (
                  <th key={i}>{renderInline(c)}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {b.rows.map((row, r) => (
                <tr key={r}>
                  {row.map((c, i) => (
                    <td key={i}>{renderInline(c)}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      );
    case "para":
      return <p key={key}>{renderInline(b.children)}</p>;
  }
}

function renderInline(nodes: Inline[]): ReactNode[] {
  return nodes.map((n, i) => {
    switch (n.type) {
      case "text":
        return <span key={i}>{n.text}</span>;
      case "code":
        return <code key={i}>{n.text}</code>;
      case "strong":
        return <strong key={i}>{renderInline(n.children)}</strong>;
      case "em":
        return <em key={i}>{renderInline(n.children)}</em>;
      case "link": {
        const href = safeHref(n.href);
        if (!href) return <span key={i}>{renderInline(n.children)}</span>;
        return (
          <a
            key={i}
            href={href}
            title={href}
            onClick={(e) => {
              e.preventDefault();
              Browser.OpenURL(href).catch(() => {});
            }}
          >
            {renderInline(n.children)}
          </a>
        );
      }
    }
  });
}

function CodeBlock({ lang, text }: { lang: string; text: string }) {
  const [copied, setCopied] = useState(false);
  const copy = () =>
    Clipboard.SetText(text)
      .catch(() => navigator.clipboard?.writeText(text))
      .then(() => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1200);
      });
  return (
    <div className="md-code">
      <div className="md-code-bar">
        <span className="muted">{lang || "text"}</span>
        <button onClick={copy} aria-label={t("markdown.copyCode")}>
          {copied ? t("markdown.copied") : t("markdown.copy")}
        </button>
      </div>
      <pre>
        <code>{text}</code>
      </pre>
    </div>
  );
}
