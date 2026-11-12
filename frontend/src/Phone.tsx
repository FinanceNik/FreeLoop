import { useEffect, useState } from "react";
import { EngineService, type Config } from "./api.ts";
import { copyText } from "./host.ts";
import { locale, t } from "./i18n.ts";
import { os } from "./platform.ts";

// Settings → Phone (GUI-052): turn on the phone server, make it reachable over
// Tailscale, pair phones, confirming each here, and see and remove them.

type PhoneStatus = Awaited<ReturnType<typeof EngineService.PhoneStatus>>;
type TailscaleInfo = Awaited<ReturnType<typeof EngineService.Tailscale>>;

interface Props {
  config: Config;
  onSaved: (c: Config) => void;
  onError: (err: unknown) => void;
}

// The tailnet port the shown command serves the phone app on (engine.PhoneHTTPSPort).
const HTTPS_PORT = 8443;

export default function Phone({ config, onSaved, onError }: Props) {
  const [status, setStatus] = useState<PhoneStatus | null>(null);
  const [tail, setTail] = useState<TailscaleInfo | null>(null);
  const [copied, setCopied] = useState("");
  const [port, setPort] = useState(String(config.remote?.port || ""));
  const [hostName, setHostName] = useState(config.remote?.host ?? "");
  useEffect(() => setHostName(config.remote?.host ?? ""), [config.remote?.host]);
  const [names, setNames] = useState<Record<string, string>>({});
  const remote = config.remote ?? { enabled: false, port: 0, host: "", keepAwake: false };

  const refresh = () => EngineService.PhoneStatus().then(setStatus).catch(onError);
  useEffect(() => {
    refresh();
    const timer = setInterval(refresh, 1500);
    return () => clearInterval(timer);
  }, []);
  const lookTailscale = () => EngineService.Tailscale().then(setTail).catch(onError);
  useEffect(() => {
    lookTailscale();
  }, [remote.port]);

  const save = (next: Partial<typeof remote>) =>
    EngineService.SaveSettings({ ...config, remote: { ...remote, ...next } })
      .then(onSaved)
      .then(refresh)
      .catch(onError);
  const copy = (key: string, text: string) =>
    copyText(text)
      .then(() => {
        setCopied(key);
        setTimeout(() => setCopied(""), 1500);
      })
      .catch(onError);

  const host = remote.host;
  const pairing = status?.pairing;
  const link = pairing && host ? `https://${host}:${HTTPS_PORT}/phone/#pair=${pairing.code}` : "";
  const time = (iso: string) => (iso ? new Date(iso).toLocaleString(locale(), { dateStyle: "medium", timeStyle: "short" }) : "–");

  let state = t("phone.state.off");
  if (status?.why === "approval") state = t("phone.state.approval");
  else if (status?.why === "local-only") state = t("phone.state.localOnly");
  else if (status?.error) state = t("phone.state.error", { error: status.error });
  else if (status?.listening) state = t("phone.state.on", { port: String(status.port) });

  return (
    <section className="page">
      <h2>{t("settings.page.phone")}</h2>
      <p className="muted">{t("phone.intro")}</p>

      <div className="card form">
        <label className="row">
          <input type="checkbox" checked={remote.enabled} onChange={(e) => save({ enabled: e.target.checked })} />
          {t("phone.enable")}
        </label>
        <p role="status">{state}</p>
        <label>
          {t("phone.port")}
          <input
            inputMode="numeric"
            value={port}
            placeholder="34217"
            onChange={(e) => setPort(e.target.value)}
            onBlur={() => {
              const n = Number(port);
              if (port === "" || (Number.isInteger(n) && n > 1023 && n < 65536)) save({ port: port === "" ? 0 : n });
            }}
          />
        </label>
        <label>
          {t("phone.host")}
          <input
            value={hostName}
            autoCapitalize="off"
            spellCheck={false}
            onChange={(e) => setHostName(e.target.value)}
            onBlur={() => hostName.trim() !== remote.host && save({ host: hostName.trim() })}
          />
          <span className="muted">{t("phone.host.note")}</span>
        </label>
        <label className="row">
          <input type="checkbox" checked={remote.keepAwake} onChange={(e) => save({ keepAwake: e.target.checked })} />
          {t("phone.keepAwake")}
        </label>
        <p className="muted">{t(os === "mac" ? "phone.keepAwake.noteMac" : "phone.keepAwake.note")}</p>
        {status?.keepingAwake && <p className="muted">{t("phone.keepingAwake")}</p>}
      </div>

      <div className="card">
        <h3>{t("phone.tailscale.title")}</h3>
        {!tail?.found ? (
          <p>{t("phone.tailscale.missing")}</p>
        ) : (
          <>
            {!tail.running && <p>{t("phone.tailscale.notRunning")}</p>}
            {tail.running && tail.name && <p>{t("phone.tailscale.name", { name: tail.name })}</p>}
            {tail.running && !tail.https && <p>{t("phone.tailscale.https")}</p>}
            <p>{t("phone.tailscale.run")}</p>
            <pre className="phone-command">{tail.command}</pre>
            <p className="row">
              <button onClick={() => copy("serve", tail.command)}>{copied === "serve" ? t("phone.copied") : t("phone.copy")}</button>
            </p>
            <p className="muted">{t("phone.tailscale.undo", { command: tail.undo })}</p>
            {os === "linux" && <p className="muted">{t("phone.tailscale.linux")}</p>}
            {tail.serving && <p className="muted">{t("phone.tailscale.serving", { port: HTTPS_PORT })}</p>}
            <p className="muted">{t("phone.tailscale.certificates")}</p>
            {tail.name && tail.name !== host && (
              <p className="row">
                <button onClick={() => save({ host: tail.name })}>{t("phone.tailscale.useName", { name: tail.name })}</button>
              </p>
            )}
          </>
        )}
        <p className="row">
          <button onClick={lookTailscale}>{t("phone.tailscale.again")}</button>
        </p>
      </div>

      <div className="card">
        <h3>{t("phone.pair.heading")}</h3>
        {!host && <p className="muted">{t("phone.pair.needsHost")}</p>}
        {!pairing ? (
          <p className="row">
            <button className="primary" disabled={!status?.listening || !host} onClick={() => EngineService.StartPhonePairing().then(refresh).catch(onError)}>
              {t("phone.pair.start")}
            </button>
          </p>
        ) : (
          <>
            <p>{t("phone.pair.open")}</p>
            <p className="phone-code">{pairing.code}</p>
            {link && (
              <p className="row">
                <code className="phone-link-text">{link}</code>
                <button onClick={() => copy("link", link)}>{copied === "link" ? t("phone.copied") : t("phone.copy")}</button>
              </p>
            )}
            <p className="muted">{t("phone.pair.until", { time: new Date(pairing.expires).toLocaleTimeString(locale(), { timeStyle: "short" }) })}</p>
            {pairing.phone ? (
              <div className="phone-confirm" role="alertdialog" aria-label={t("phone.pair.confirmTitle")}>
                <p>{pairing.login ? t("phone.pair.askLogin", { phone: pairing.phone, login: pairing.login }) : t("phone.pair.ask", { phone: pairing.phone })}</p>
                <p className="row">
                  <button className="primary" onClick={() => EngineService.AnswerPhonePairing(true).then(refresh).catch(onError)}>
                    {t("phone.pair.allow")}
                  </button>
                  <button onClick={() => EngineService.AnswerPhonePairing(false).then(refresh).catch(onError)}>{t("phone.pair.refuse")}</button>
                </p>
              </div>
            ) : (
              <p className="row">
                <button onClick={() => EngineService.CancelPhonePairing().then(refresh).catch(onError)}>{t("common.cancel")}</button>
              </p>
            )}
          </>
        )}
      </div>

      <div className="card">
        <h3>{t("phone.devices")}</h3>
        {(status?.devices ?? []).length === 0 ? (
          <p className="muted">{t("phone.devices.none")}</p>
        ) : (
          <>
            <ul className="phone-devices">
              {(status?.devices ?? []).map((d) => (
                <li key={d.id}>
                  <input
                    aria-label={t("phone.devices.name")}
                    value={names[d.id] ?? d.name}
                    onChange={(e) => setNames({ ...names, [d.id]: e.target.value })}
                    onBlur={() => names[d.id] !== undefined && names[d.id] !== d.name && EngineService.RenamePhone(d.id, names[d.id]).then(refresh).catch(onError)}
                  />
                  <span className="muted">
                    {d.login ? `${d.login} · ` : ""}
                    {d.notify ? `${t("phone.devices.notify")} · ` : ""}
                    {t("phone.devices.seen", { time: time(d.lastSeen) })}
                  </span>
                  <button onClick={() => EngineService.RemovePhone(d.id).then(refresh).catch(onError)}>{t("common.remove")}</button>
                </li>
              ))}
            </ul>
            <p className="row">
              <button onClick={() => EngineService.RemovePhone("").then(refresh).catch(onError)}>{t("phone.devices.signOutAll")}</button>
            </p>
          </>
        )}
      </div>
      <p className="muted">{t("phone.note")}</p>
    </section>
  );
}
