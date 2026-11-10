// The errors the window shows. Parallel sessions fail at the same time, so a new
// error must not replace one the user has not read, and each says which session
// it came from. Pure, so errorlist.test.ts covers it.
import { t } from "./i18n.ts";

const KEEP = 4;

// An error as shown: the engine's words, as given (GUI-048), and the session it
// came from, if any.
export interface ShownError {
  detail: string;
  session?: string;
}

// errorHeading is what stands before an error's words, in the language in force;
// written where it is shown, so a change of language reaches errors already listed.
export function errorHeading(e: ShownError): string {
  return e.session ? t("app.error.inSession", { title: e.session }) : t("app.error.heading");
}

export function addError(list: readonly ShownError[], e: ShownError): ShownError[] {
  return [...list.filter((l) => l.detail !== e.detail || l.session !== e.session), e].slice(-KEEP);
}
