// Words for scheduled sessions (SES-012). Pure, so schedules.test.ts
// covers them.
import { locale, t, type PlainKey } from "./i18n.ts";

export const REPEATS: readonly { id: string; name: PlainKey }[] = [
  { id: "daily", name: "schedules.repeat.daily" },
  { id: "weekdays", name: "schedules.repeat.weekdays" },
  { id: "weekly", name: "schedules.repeat.weekly" },
  { id: "hourly", name: "schedules.repeat.hourly" },
];

// weekday names a day by its number, Sunday 0 as a schedule counts them, in the
// language in force.
export function weekday(day: number): string {
  // 7 January 2024 was a Sunday.
  return new Intl.DateTimeFormat(locale(), { weekday: "long" }).format(new Date(2024, 0, 7 + day));
}

// weekdays are the days' names from Sunday, for a weekly schedule's picker.
export function weekdays(): string[] {
  return [0, 1, 2, 3, 4, 5, 6].map(weekday);
}

// describe says when a schedule runs.
export function describe(s: { repeat: string; at: string; weekday?: number }): string {
  switch (s.repeat) {
    case "hourly":
      return t("schedules.when.hourly", { minute: s.at.slice(3, 5) });
    case "weekdays":
      return t("schedules.when.weekdays", { time: s.at });
    case "weekly":
      return t("schedules.when.weekly", { weekday: weekday(s.weekday ?? 0), time: s.at });
    default:
      return t("schedules.when.daily", { time: s.at });
  }
}

// when shows a time relative to now: today and tomorrow by name, a date
// within the week by weekday, anything else by date.
export function when(time: string | Date | undefined, now: Date = new Date()): string {
  if (!time) return "";
  const d = new Date(time);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return "";
  const hm = d.toLocaleTimeString(locale(), { hour: "2-digit", minute: "2-digit" });
  const day = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const diff = Math.round((day(d) - day(now)) / 86400000);
  if (diff === 0) return t("schedules.at.today", { time: hm });
  if (diff === 1) return t("schedules.at.tomorrow", { time: hm });
  if (diff === -1) return t("schedules.at.yesterday", { time: hm });
  if (diff > 1 && diff < 7) return `${weekday(d.getDay())} ${hm}`;
  return `${d.toLocaleDateString(locale())} ${hm}`;
}
