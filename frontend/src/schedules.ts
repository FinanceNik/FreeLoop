// Words for scheduled sessions (SES-012). Pure, so schedules.test.ts
// covers them.

export const REPEATS = [
  { id: "daily", name: "Every day" },
  { id: "weekdays", name: "Weekdays" },
  { id: "weekly", name: "Every week" },
  { id: "hourly", name: "Every hour" },
];

export const WEEKDAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

// describe says when a schedule runs.
export function describe(s: { repeat: string; at: string; weekday?: number }): string {
  switch (s.repeat) {
    case "hourly":
      return `Every hour at :${s.at.slice(3, 5)}`;
    case "weekdays":
      return `Weekdays at ${s.at}`;
    case "weekly":
      return `${WEEKDAYS[s.weekday ?? 0]}s at ${s.at}`;
    default:
      return `Every day at ${s.at}`;
  }
}

// when shows a time relative to now: today and tomorrow by name, a date
// within the week by weekday, anything else by date.
export function when(t: string | Date | undefined, now: Date = new Date()): string {
  if (!t) return "";
  const d = new Date(t);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return "";
  const hm = d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  const day = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const diff = Math.round((day(d) - day(now)) / 86400000);
  if (diff === 0) return `today ${hm}`;
  if (diff === 1) return `tomorrow ${hm}`;
  if (diff === -1) return `yesterday ${hm}`;
  if (diff > 1 && diff < 7) return `${WEEKDAYS[d.getDay()]} ${hm}`;
  return `${d.toLocaleDateString()} ${hm}`;
}
