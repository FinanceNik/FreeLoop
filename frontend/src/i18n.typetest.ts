// Checked by tsc, never run (GUI-048): a translation that misses a key, adds one or
// changes a {placeholder} fails the type check, and so does a call to t without the
// values its text needs. Each @ts-expect-error fails tsc if its line stops being an
// error, so this file breaks when the catalogues stop being held to English.
import { t } from "./i18n.ts";
import { translationOf } from "./i18n/catalogue.ts";

const source = { hello: "Hello {name}", files: { one: "{count} file", other: "{count} files" }, plain: "Save" } as const;
const translation = translationOf<typeof source>();

translation({ hello: "Hallo {name}", files: { one: "eine Datei", other: "{count} Dateien" }, plain: "Sichern" });
// @ts-expect-error a key missing
translation({ hello: "Hallo {name}", files: { one: "{count} Datei", other: "{count} Dateien" } });
// @ts-expect-error a key that English does not have
translation({ hello: "Hallo {name}", files: { one: "{count} Datei", other: "{count} Dateien" }, plain: "Sichern", extra: "Mehr" });
// @ts-expect-error a placeholder renamed
translation({ hello: "Hallo {nom}", files: { one: "{count} Datei", other: "{count} Dateien" }, plain: "Sichern" });
// @ts-expect-error a placeholder dropped
translation({ hello: "Hallo", files: { one: "{count} Datei", other: "{count} Dateien" }, plain: "Sichern" });
// @ts-expect-error a placeholder added
translation({ hello: "Hallo {name}", files: { one: "{count} Datei", other: "{count} Dateien" }, plain: "Sichern {name}" });
// @ts-expect-error a plural's count dropped from every form
translation({ hello: "Hallo {name}", files: { one: "eine Datei", other: "Dateien" }, plain: "Sichern" });
// @ts-expect-error text where English has plural forms
translation({ hello: "Hallo {name}", files: "{count} Dateien", plain: "Sichern" });
// @ts-expect-error plural forms where English has text
translation({ hello: "Hallo {name}", files: { one: "{count} Datei", other: "{count} Dateien" }, plain: { one: "Sichern", other: "Sichern" } });

t("settings.updates.available", { version: "1.0" });
t("settings.network.blocked", { count: 2 });
t("common.save");
// @ts-expect-error a placeholder's value missing
t("settings.updates.available");
// @ts-expect-error a value for a placeholder the text does not have
t("settings.updates.available", { version: "1.0", other: 1 });
// @ts-expect-error a plural without its count
t("settings.network.blocked", {});
// @ts-expect-error a key no catalogue has
t("settings.nothing");
