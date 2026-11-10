import { useEffect, useMemo, useRef, useState } from "react";
import { Markdown } from "./Markdown.tsx";
import { helpTopics, searchHelp, type HelpPlace } from "./help.ts";
import { language, t, type Language } from "./i18n.ts";
import { forPlatform } from "./platform.ts";

// The help pages (GUI-047): Markdown files in src/help/, built into the app, so they
// need no network and match the version installed. They go through the same parser as
// an agent's replies, which never lets markup reach the page as HTML. English pages
// are in src/help/, German and Dutch ones in src/help/de/ and src/help/nl/ (GUI-048).
const files = import.meta.glob("./help/**/*.md", { query: "?raw", import: "default", eager: true }) as Record<string, string>;
// Written for a Mac, they are adapted to the system they are read on (forPlatform):
// Ctrl for ⌘ on Windows and Linux, and {{mac|windows|linux}} words where they differ.
const PAGES: Record<string, Record<string, string>> = {};
for (const [path, text] of Object.entries(files)) {
  const m = path.match(/^\.\/help\/(?:(de|nl)\/)?([^/]+)\.md$/);
  if (m) (PAGES[m[1] ?? "en"] ??= {})[m[2]] = forPlatform(text);
}

// helpPages is the pages in the language in force, each falling back to English.
export function helpPages(lang: Language = language()): Record<string, string> {
  return { ...PAGES.en, ...PAGES[lang] };
}

// body is a page without its first-line title, which the screen shows as the page's
// heading: the Markdown parser lowers headings for an agent's replies.
const body = (page: string) => page.replace(/^#\s.*\n+/, "");

// layout splits a page into its lead (the text before the first section), the rest,
// and the sections' titles, for the "On this page" links.
function layout(page: string): { lead: string; rest: string; sections: string[] } {
  const b = body(page);
  const i = b.search(/^## /m);
  const lead = (i < 0 ? b : b.slice(0, i)).trim();
  const rest = i < 0 ? "" : b.slice(i);
  const sections = [...rest.matchAll(/^## (.+)$/gm)].map((m) => m[1].replace(/[*`_]/g, "").trim());
  return { lead, rest, sections };
}

// shownTopics is the topics there is a page for, in reading order, in the language
// in force.
export function shownTopics() {
  const pages = helpPages();
  return helpTopics().filter((x) => pages[x.slug] !== undefined);
}

export default function Help({ topic, onTopic, onOpen }: { topic: string; onTopic: (slug: string) => void; onOpen: (place: HelpPlace) => void }) {
  const [query, setQuery] = useState("");
  // A change of language re-renders the app, and the page follows it.
  const lang = language();
  const pages = useMemo(() => helpPages(lang), [lang]);
  const topics = useMemo(() => shownTopics(), [lang]);
  const hits = useMemo(() => searchHelp(topics, pages, query), [topics, pages, query]);
  const at = Math.max(0, topics.findIndex((x) => x.slug === topic));
  const current = topics[at];
  const page = useRef<HTMLElement>(null);
  const shown = useMemo(() => (current ? layout(pages[current.slug]) : { lead: "", rest: "", sections: [] }), [current, pages]);
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
  const prev = topics[at - 1];
  const next = topics[at + 1];

  return (
    <section className="help" aria-label={t("help.title")}>
      <nav className="help-nav" aria-label={t("help.topics")}>
        <h2>{t("help.title")}</h2>
        <input type="search" placeholder={t("help.search")} aria-label={t("help.search")} value={query} onChange={(e) => setQuery(e.target.value)} />
        {query.trim() ? (
          hits.length > 0 ? (
            <ul>{hits.map((h) => link(h.slug, h.title, h.snippet))}</ul>
          ) : (
            <p className="muted help-none">{t("help.noMatch")}</p>
          )
        ) : (
          <ul>{topics.map((x) => link(x.slug, x.title))}</ul>
        )}
      </nav>
      <article className="help-page" ref={page} tabIndex={-1}>
        {current ? (
          <>
            <h1>{current.title}</h1>
            {shown.lead && (
              <div className="help-lead">
                <Markdown text={shown.lead} />
              </div>
            )}
            {shown.sections.length > 2 && (
              <nav className="help-toc" aria-label={t("help.onThisPage")}>
                {shown.sections.map((s, i) => (
                  <button type="button" className="link" key={s} onClick={() => page.current?.querySelectorAll(".help-body h4")[i]?.scrollIntoView({ block: "start", behavior: "smooth" })}>
                    {s}
                  </button>
                ))}
              </nav>
            )}
            <div className="help-body">
              <Markdown text={shown.rest} />
            </div>
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
          <p className="muted">{t("help.noPages")}</p>
        )}
      </article>
    </section>
  );
}
