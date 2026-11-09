import { t } from "./i18n.ts";

// The message box's placeholder. An empty box's height does not count its placeholder,
// so a pane the user is not in, where the box is narrow and one line, gets the short
// form, whole.
export function placeholder(running: boolean, inPane: boolean): string {
  if (running) return t(inPane ? "composer.placeholder.queue" : "composer.placeholder.queueShort");
  return t(inPane ? "composer.placeholder" : "composer.placeholder.short");
}
