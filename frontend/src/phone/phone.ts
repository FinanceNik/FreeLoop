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
