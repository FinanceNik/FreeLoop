// The message box's placeholder. An empty box's height does not count its placeholder,
// so a pane the user is not in, where the box is narrow and one line, gets the short
// form, whole.
export function placeholder(running: boolean, inPane: boolean): string {
  if (running) return inPane ? "Queue a message for when this turn ends" : "Queue a message";
  return inPane ? "Ask for a change, a fix, an explanation… Type @ to attach a file, / for commands." : "Ask for a change…";
}
