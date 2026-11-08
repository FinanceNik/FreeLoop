import { Dialogs } from "@wailsio/runtime";
import { t } from "./i18n.ts";

// ask puts a question to the user in a native dialog. The page's own confirm() must
// not be used: Wails gives the web view no way to show it, so in the packaged app it
// answers no without showing anything. A build served to a browser has no native
// dialogs, and there confirm() works.
export async function ask(message: string): Promise<boolean> {
  const ok = t("common.ok");
  try {
    return (await Dialogs.Question({ Message: message, Buttons: [{ Label: t("common.cancel"), IsCancel: true }, { Label: ok, IsDefault: true }] })) === ok;
  } catch {
    return window.confirm(message);
  }
}
