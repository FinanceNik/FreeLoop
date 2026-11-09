// The app preview's address (GUI-020). Only this computer's own addresses load,
// never the app's own page. Pure, so preview.test.ts covers it.
import { t } from "./i18n.ts";

const LOCAL = new Set(["localhost", "127.0.0.1", "[::1]"]);

export interface PreviewAddress {
  url: string;
  port: number;
}

// parseAddress reads what the user typed: 3000, localhost:3000 or a whole
// address. It returns the address, or why it will not load.
export function parseAddress(input: string, appOrigin: string): PreviewAddress | { error: string } {
  const text = input.trim();
  if (!text) return { error: t("preview.error.empty") };
  const raw = /^\d+$/.test(text) ? `http://localhost:${text}` : /^[a-z][a-z0-9+.-]*:\/\//i.test(text) ? text : `http://${text}`;
  let u: URL;
  try {
    u = new URL(raw);
  } catch {
    return { error: t("preview.error.notAddress", { text }) };
  }
  if (u.protocol !== "http:" && u.protocol !== "https:") return { error: t("preview.error.scheme") };
  if (!LOCAL.has(u.hostname)) return { error: t("preview.error.notLocal") };
  if (u.username || u.password) return { error: t("preview.error.credentials") };
  if (u.origin === appOrigin) return { error: t("preview.error.ownPage") };
  const port = u.port ? Number(u.port) : u.protocol === "https:" ? 443 : 80;
  if (!(port >= 1 && port <= 65535)) return { error: t("preview.error.noPort") };
  return { url: u.href, port };
}

// framed reports a page shown inside another, which the app's own page never is.
export function framed(w: { top: unknown; self: unknown }): boolean {
  return w.top !== w.self;
}
