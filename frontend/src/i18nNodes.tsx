import { Fragment, type ReactNode } from "react";
import { tParts, type Key, type Vars } from "./i18n.ts";

// tNodes is t for text with an element in it, such as code or a link (GUI-048): each
// placeholder's value, text or element, in its place, so a language can put it where
// its word order needs it.
export function tNodes<K extends Key>(key: K, vars: Vars<K, ReactNode>): ReactNode {
  return tParts<K, ReactNode>(key, ...([vars] as never)).map((p, i) => <Fragment key={i}>{p}</Fragment>);
}
