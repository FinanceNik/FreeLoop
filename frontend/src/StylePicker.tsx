import { useState } from "react";
import { t } from "./i18n.ts";
import { EngineService, type Style } from "./api.ts";
import { ask } from "./ask.ts";
import { inFileManager, showInFiles } from "./platform.ts";
import { isChanged, selectedStyle, type Look } from "./styles.ts";

// The styles of Settings → Appearance (GUI-053): saved sets of the fonts and
// colors below, one file each in the styles folder. Choosing one copies it over
// the fonts and colors in force; changing those afterwards marks it changed,
// and only Update writes the change into the style.

interface Props {
  styles: Style[];
  // What is on screen: the colors in force and the fonts in config.ui.
  look: Look;
  // ui.style: the style chosen last, or "" for the app's own look.
  chosen: string;
  // onApply switches to a style, or to the app's own look with null; it asks
  // first when the look on screen is not saved, unless confirm is false.
  onApply: (style: Style | null, confirm?: boolean) => void;
  // onChoose sets ui.style alone, once the look on screen is a style's.
  onChoose: (id: string) => void;
  onChanged: () => void;
  onError: (err: unknown) => void;
}

export default function StylePicker({ styles, look, chosen, onApply, onChoose, onChanged, onError }: Props) {
  const style = selectedStyle(styles, chosen);
  const changed = isChanged(look, style);
  // The name of a new style, while it is typed.
  const [name, setName] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const saveNew = () => {
    const n = name?.trim();
    if (!n || busy) return;
    setBusy(true);
    EngineService.SaveStyle({ id: "", version: 1, name: n, ...look })
      .then((s) => {
        setName(null);
        onChanged();
        onChoose(s.id);
      })
      .catch(onError)
      .finally(() => setBusy(false));
  };
  const update = () => {
    if (style) EngineService.SaveStyle({ ...style, ...look }).then(onChanged).catch(onError);
  };
  const remove = async () => {
    if (!style || !(await ask(t("styles.deleteAsk", { name: style.name })))) return;
    EngineService.DeleteStyle(style.id)
      .then(() => {
        onChanged();
        onChoose("");
      })
      .catch(onError);
  };
  const label = (n: string, on: boolean) => (on && changed ? t("styles.changed", { name: n }) : n);

  return (
    <div className="style-picker">
      <p className="muted">{t("styles.intro")}</p>
      <label>
        {t("styles.style")}
        <select className="style-select" value={style?.id ?? ""} onChange={(e) => onApply(selectedStyle(styles, e.target.value))}>
          <option value="">{label(t("styles.own"), !style)}</option>
          {styles.map((s) => (
            <option key={s.id} value={s.id}>
              {label(s.name, s === style)}
            </option>
          ))}
        </select>
      </label>
      {style && changed && <p className="muted">{t("styles.changedNote")}</p>}
      <div className="row wrap">
        {name === null ? (
          <button type="button" onClick={() => setName("")}>
            {t("styles.saveNew")}
          </button>
        ) : (
          <>
            <input
              type="text"
              className="style-name"
              aria-label={t("styles.name")}
              placeholder={t("styles.namePlaceholder")}
              maxLength={64}
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  saveNew();
                } else if (e.key === "Escape") {
                  e.preventDefault();
                  e.stopPropagation();
                  setName(null);
                }
              }}
            />
            <button type="button" className="primary" disabled={busy || !name.trim()} onClick={saveNew}>
              {t("common.save")}
            </button>
            <button type="button" onClick={() => setName(null)}>
              {t("common.cancel")}
            </button>
          </>
        )}
        {style && changed && (
          <>
            <button type="button" onClick={update}>
              {t("styles.update")}
            </button>
            <button type="button" onClick={() => onApply(style, false)}>
              {t("common.revert")}
            </button>
          </>
        )}
        {style && (
          <button type="button" onClick={remove}>
            {t("styles.delete")}
          </button>
        )}
        <button type="button" title={t("styles.folder", { where: inFileManager() })} onClick={() => EngineService.ShowStyles().catch(onError)}>
          {showInFiles()}
        </button>
      </div>
    </div>
  );
}
