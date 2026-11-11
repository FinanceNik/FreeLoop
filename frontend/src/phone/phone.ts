// What the phone app (GUI-052) works out without the network. Pure, so node --test
// covers it.
import type { Language } from "../i18n.ts";

// A request as the phone summary sends it.
export interface PhoneOption {
  optionId: string;
  name: string;
  kind: string;
}

// onceOptions are the answers the phone may give: allow or deny once. Anything
// else, as "always" or a plan's mode, is answered on the computer.
export function onceOptions(options: readonly PhoneOption[] | null | undefined): { allow?: PhoneOption; deny?: PhoneOption } {
  const list = options ?? [];
  return { allow: list.find((o) => o.kind === "allow_once"), deny: list.find((o) => o.kind === "reject_once") };
}

// pairCode reads the code a pairing link carries after #pair=.
export function pairCode(hash: string): string {
  const m = /(?:^#|&)pair=([A-Za-z0-9-]+)/.exec(hash);
  return m ? m[1] : "";
}

// deviceName is a first name for this phone, from what the browser says it is.
export function deviceName(userAgent: string): string {
  if (/iPhone/.test(userAgent)) return "iPhone";
  if (/iPad/.test(userAgent)) return "iPad";
  if (/Android/.test(userAgent)) return "Android phone";
  return "Phone";
}

// pickLanguage is the first of the phone's languages the app speaks.
export function pickLanguage(preferred: readonly string[]): Language {
  for (const tag of preferred) {
    const lang = tag.toLowerCase().split(/[-_]/)[0];
    if (lang === "en" || lang === "de" || lang === "nl") return lang;
  }
  return "en";
}

// A server-sent message: its id and event name, and its data lines joined by newlines.
export interface SSEMessage {
  id: string;
  event: string;
  data: string;
}

// parseSSE reads the complete messages in what has come so far (rest, the unfinished
// end the last call left, plus chunk) and returns them with the new unfinished end.
// The phone reads its streams with fetch, so that its key goes as a header: the
// browser's EventSource cannot send one.
export function parseSSE(rest: string, chunk: string): { messages: SSEMessage[]; rest: string } {
  const text = (rest + chunk).replace(/\r\n?/g, "\n");
  const parts = text.split("\n\n");
  const unfinished = parts.pop() ?? "";
  const messages: SSEMessage[] = [];
  for (const part of parts) {
    let id = "";
    let event = "";
    const data: string[] = [];
    for (const line of part.split("\n")) {
      if (line === "" || line.startsWith(":")) continue;
      const i = line.indexOf(":");
      const field = i < 0 ? line : line.slice(0, i);
      const value = i < 0 ? "" : line.slice(i + 1).replace(/^ /, "");
      if (field === "id") id = value;
      else if (field === "event") event = value;
      else if (field === "data") data.push(value);
    }
    if (data.length > 0) messages.push({ id, event: event || "message", data: data.join("\n") });
  }
  return { messages, rest: unfinished };
}

// reconnectDelay is how long to wait, in milliseconds, before the nth try to take a
// dropped stream up again: half a second, doubling, up to 15 seconds.
export function reconnectDelay(attempt: number): number {
  return Math.min(500 * 2 ** attempt, 15000);
}
