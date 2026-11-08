import { useEffect, useMemo, useRef, useState } from "react";
import { Markdown } from "./Markdown.tsx";
import { HELP_TOPICS, searchHelp, type HelpPlace } from "./help.ts";

// The help pages (GUI-047): Markdown files in src/help/, built into the app, so they
// need no network and match the version installed. They go through the same parser as
// an agent's replies, which never lets markup reach the page as HTML.
const files = import.meta.glob("./help/*.md", { query: "?raw", import: "default", eager: true }) as Record<string, string>;
export const HELP_PAGES: Record<string, string> = Object.fromEntries(Object.entries(files).map(([path, text]) => [path.replace(/^.*\/([^/]+)\.md$/, "$1"), text]));

// body is a page without its first-line title, which the screen shows as the page's
// heading: the Markdown parser lowers headings for an agent's replies.
const body = (page: string) => page.replace(/^#\s.*\n+/, "");

// The topics there is a page for, in reading order.
export const SHOWN_TOPICS = HELP_TOPICS.filter((t) => HELP_PAGES[t.slug] !== undefined);

export default function Help({ topic, onTopic, onOpen }: { topic: string; onTopic: (slug: string) => void; onOpen: (place: HelpPlace) => void }) {
  const [query, setQuery] = useState("");
  const hits = useMemo(() => searchHelp(SHOWN_TOPICS, HELP_PAGES, query), [query]);
  const at = Math.max(0, SHOWN_TOPICS.findIndex((t) => t.slug === topic));
  const current = SHOWN_TOPICS[at];
  const page = useRef<HTMLElement>(null);
  // A new topic is read from its top.
  useEffect(() => page.current?.scrollTo({ top: 0 }), [current?.slug]);

  const link = (slug: string, title: string, extra?: string) => (
    <li key={slug}>
      <button type="button" className="help-topic" aria-current={slug === current?.slug ? "page" : undefined} onClick={() => onTopic(slug)}>
        <span className="help-topic-title">{title}</span>
        {extra && <span className="help-snippet">{extra}</span>}
      </button>
    </li>
  );
  const prev = SHOWN_TOPICS[at - 1];
  const next = SHOWN_TOPICS[at + 1];

  return (
    <section className="help" aria-label="Help">
      <nav className="help-nav" aria-label="Help topics">
        <h2>Help</h2>
        <input type="search" placeholder="Search the help" aria-label="Search the help" value={query} onChange={(e) => setQuery(e.target.value)} />
        {query.trim() ? (
          hits.length > 0 ? (
            <ul>{hits.map((h) => link(h.slug, h.title, h.snippet))}</ul>
          ) : (
            <p className="muted help-none">Nothing in the help matches.</p>
          )
        ) : (
          <ul>{SHOWN_TOPICS.map((t) => link(t.slug, t.title))}</ul>
        )}
      </nav>
      <article className="help-page" ref={page} tabIndex={-1}>
        {current ? (
          <>
            <h1>{current.title}</h1>
            <Markdown text={body(HELP_PAGES[current.slug])} />
            <div className="help-opens row">
              {current.opens.map((o) => (
                <button type="button" key={o.label} onClick={() => onOpen(o.place)}>
                  {o.label}
                </button>
              ))}
            </div>
            <div className="help-turn">
              {prev ? (
                <button type="button" className="link" onClick={() => onTopic(prev.slug)}>
                  ← {prev.title}
                </button>
              ) : (
                <span />
              )}
              {next && (
                <button type="button" className="link" onClick={() => onTopic(next.slug)}>
                  {next.title} →
                </button>
              )}
            </div>
          </>
        ) : (
          <p className="muted">There are no help pages in this build.</p>
        )}
      </article>
    </section>
  );
}
