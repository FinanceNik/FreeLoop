import { useEffect, useState } from "react";
import { EngineService, type Config, type VoiceStatus } from "./api.ts";
import DictateButton, { size, useVoiceDownload } from "./Dictation.tsx";
import { LANGUAGES, t } from "./i18n.ts";

// The Settings page for voice input (GUI-050): the speech model, which language
// speech is in, and a place to try it.

interface Props {
  config: Config;
  onSaved: (c: Config) => void;
  onError: (err: unknown) => void;
}

export default function Voice({ config, onSaved, onError }: Props) {
  const [status, setStatus] = useState<VoiceStatus | null>(null);
  const [heard, setHeard] = useState("");
  const [said, setSaid] = useState("");
  const refresh = () => EngineService.VoiceStatus().then(setStatus).catch(onError);
  useEffect(() => {
    refresh();
  }, []);
  useVoiceDownload(status, setStatus);
  const download = () => EngineService.InstallVoice().then(refresh).catch(onError);
  const stop = () => EngineService.CancelVoiceInstall().then(refresh).catch(onError);
  const remove = () => EngineService.RemoveVoice().then(refresh).catch(onError);
  const setLanguage = (language: string) =>
    EngineService.SaveSettings({ ...config, voice: { ...config.voice, language } })
      .then(onSaved)
      .catch(onError);

  const state = status?.state;
  return (
    <section className="page">
      <h2>{t("settings.page.voice")}</h2>
      <p className="muted">{t("voice.intro")}</p>
      <div className="card">
        <h3>{status ? status.modelName : t("voice.title")}</h3>
        {status && (
          <p className="muted">
            {t("voice.about", { version: status.version, license: status.license })}
          </p>
        )}
        <p role="status">
          {state === "ready" && t("voice.state.ready")}
          {state === "missing" && t("voice.state.missing", { size: size(status!.download) })}
          {state === "downloading" && t("voice.dialog.received", { received: size(status!.received), total: size(status!.total) })}
          {state === "failed" && t("voice.state.failed", { error: status!.error })}
          {state === "unsupported" && t("voice.state.unsupported")}
        </p>
        {state === "downloading" && <progress className="voice-progress" max={status!.total} value={status!.received} aria-label={t("voice.dialog.progress")} />}
        <p className="row">
          {(state === "missing" || state === "failed") && (
            <button className="primary" onClick={download}>
              {t("voice.dialog.download")}
            </button>
          )}
          {state === "downloading" && <button onClick={stop}>{t("voice.dialog.stop")}</button>}
          {state === "ready" && <button onClick={remove}>{t("common.remove")}</button>}
        </p>
        <p className="muted">
          {t("voice.dialog.where")} {status && t("voice.folder", { folder: status.folder })}
        </p>
      </div>
      <div className="card form">
        <label>
          {t("voice.language")}
          <select value={config.voice?.language ?? ""} onChange={(e) => setLanguage(e.target.value)}>
            <option value="">{t("voice.language.window")}</option>
            <option value="auto">{t("voice.language.auto")}</option>
            {/* Each named in itself, as in the window's language picker. */}
            {LANGUAGES.map((l) => (
              <option key={l.id} value={l.id}>
                {l.name}
              </option>
            ))}
          </select>
          <span className="muted">{t("voice.language.note")}</span>
        </label>
      </div>
      {state === "ready" && (
        <div className="card">
          <h3>{t("voice.try")}</h3>
          <p className="muted">{t("voice.try.how")}</p>
          <p className="row voice-try">
            <DictateButton active={false} onText={setHeard} onAnnounce={setSaid} />
            <span className="voice-heard">{heard}</span>
          </p>
          <span className="sr-only" role="status">
            {said}
          </span>
        </div>
      )}
    </section>
  );
}
