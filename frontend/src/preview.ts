// The app preview's address (GUI-020). Only this computer's own addresses load,
// never the app's own page. Pure, so preview.test.ts covers it.

const LOCAL = new Set(["localhost", "127.0.0.1", "[::1]"]);

export interface PreviewAddress {
  url: string;
  port: number;
}

// parseAddress reads what the user typed: 3000, localhost:3000 or a whole
// address. It returns the address, or why it will not load.
export function parseAddress(input: string, appOrigin: string): PreviewAddress | { error: string } {
  const text = input.trim();
  if (!text) return { error: "Enter a port or a localhost address." };
  const raw = /^\d+$/.test(text) ? `http://localhost:${text}` : /^[a-z][a-z0-9+.-]*:\/\//i.test(text) ? text : `http://${text}`;
  let u: URL;
  try {
    u = new URL(raw);
  } catch {
    return { error: `"${text}" is not an address.` };
  }
  if (u.protocol !== "http:" && u.protocol !== "https:") return { error: "Only http and https addresses load." };
  if (!LOCAL.has(u.hostname)) return { error: "The preview shows this computer's own servers only; open anything else in your browser." };
  if (u.username || u.password) return { error: "An address with a user name or password does not load." };
  if (u.origin === appOrigin) return { error: "That is Freeloop's own page." };
  const port = u.port ? Number(u.port) : u.protocol === "https:" ? 443 : 80;
  if (!(port >= 1 && port <= 65535)) return { error: "That port does not exist." };
  return { url: u.href, port };
}

// framed reports a page shown inside another, which the app's own page never is.
export function framed(w: { top: unknown; self: unknown }): boolean {
  return w.top !== w.self;
}
