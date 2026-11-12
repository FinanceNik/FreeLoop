import { Clipboard } from "@wailsio/runtime";
import { useEffect, useEffectEvent, useState } from "react";
import { EngineService, type Config, type OptimizerCheck, type OptimizerStatus } from "./api.ts";
import { t } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";

// The Settings page for token optimizers (HRN-019): which are installed, and
// what new sessions use. Listing them runs nothing, since another program can
// have the same name; Check and choosing one do. A file's optimizer also has to be
// approved first, as MCP servers are, since it arrives without asking.

interface Props {
  config: Config;
  // Rises when a config file changes, such as a file in optimizers/.
  version: number;
  onSaved: (c: Config) => void;
  onError: (err: unknown) => void;
}

export default function Optimizers({ config, version, onSaved, onError }: Props) {
  const [list, setList] = useState<OptimizerStatus[] | null>(null);
  const [checks, setChecks] = useState<Record<string, OptimizerCheck>>({});
  const [copied, setCopied] = useState("");
  const refresh = () =>
    EngineService.ListOptimizers()
      .then((l) => setList(l ?? []))
      .catch(onError);
  const refreshNow = useEffectEvent(refresh);
  useEffect(() => {
    refreshNow();
  }, [version]);

  const check = (id: string) =>
    EngineService.CheckOptimizer(id)
      .then((c) => {
        setChecks((old) => ({ ...old, [id]: c }));
        refresh();
      })
      .catch(onError);
  const approve = (o: OptimizerStatus) =>
    EngineService.ApproveOptimizer(o.id, o.fingerprint)
      .then((c) => {
        onSaved(c);
        refresh();
      })
      .catch(onError);
  const [installing, setInstalling] = useState("");
  const install = (o: OptimizerStatus) => {
    setInstalling(o.id);
    EngineService.InstallOptimizer(o.id)
      .then(refresh)
      .catch(onError)
      .finally(() => setInstalling(""));
  };
  const remove = (o: OptimizerStatus) => EngineService.RemoveOptimizer(o.id).then(refresh).catch(onError);
  const setDefault = (id: string) => EngineService.SetDefaultOptimizer(id).then(onSaved).catch(onError);
  const copy = (id: string, text: string) =>
    Clipboard.SetText(text)
      .then(() => {
        setCopied(id);
        setTimeout(() => setCopied(""), 1500);
      })
      .catch(onError);

  const chosen = config.defaults.optimizer;
  const [waitsApproval, setWaitsApproval] = useState(false);
  const configKey = JSON.stringify(config);
  useEffect(() => {
    EngineService.StartWaiting("")
      .then((w) => setWaitsApproval(w.optimizer && !!chosen))
      .catch(() => setWaitsApproval(false));
  }, [configKey, chosen]);
  return (
    <section className="page">
      <h2>{t("settings.page.optimizers")}</h2>
      <p className="muted">{t("optimizers.intro")}</p>
      <div className="card form">
        <label>
          {t("optimizers.forNew")}
          <select value={chosen} onChange={(e) => setDefault(e.target.value)}>
            <option value="">{t("optimizers.off")}</option>
            {(list ?? []).map((o) => (
              <option key={o.id} value={o.id}>
                {o.needsApproval ? t("optimizers.option.notApproved", { name: o.name }) : o.path ? o.name : t("optimizers.option.notInstalled", { name: o.name })}
              </option>
            ))}
            {chosen && list && !list.some((o) => o.id === chosen) && <option value={chosen}>{t("optimizers.option.unknown", { name: chosen })}</option>}
          </select>
          <span className="muted">{t("optimizers.profileNote")}</span>
          {waitsApproval && <span className="muted">{t("optimizers.waits")}</span>}
        </label>
      </div>
      {list === null && <p className="muted">{t("optimizers.looking")}</p>}
      {(list ?? []).map((o) => {
        const c = checks[o.id];
        return (
          <div className="card" key={o.id}>
            <h3>{o.name}</h3>
            <p className="muted">
              {o.source ? `${t("optimizers.fromSource", { source: o.source })} ` : ""}
              {o.path ? t("optimizers.foundAt", { path: o.path }) : t("optimizers.notInstalled")}
            </p>
            {o.needsApproval && (
              <>
                <p>{tNodes("optimizers.notApproved", { runs: <code>{o.runs}</code> })}</p>
                <p className="row">
                  <button onClick={() => approve(o)}>{t("optimizers.approve")}</button>
                </p>
              </>
            )}
            {!o.path && o.canDownload && !o.needsApproval && (
              <>
                <p className="muted">{o.download}</p>
                <p className="row">
                  <button className="primary" disabled={installing === o.id} onClick={() => install(o)}>
                    {installing === o.id ? t("optimizers.installing") : t("optimizers.install", { name: o.name })}
                  </button>
                </p>
              </>
            )}
            {o.downloaded && (
              <p className="row">
                <button onClick={() => remove(o)}>{t("optimizers.removeCopy")}</button>
              </p>
            )}
            {!o.path && o.install && !o.needsApproval && (
              <p className="row">
                <code>{o.install}</code>
                <button onClick={() => copy(o.id, o.install)}>{copied === o.id ? t("optimizers.copied") : t("optimizers.copy")}</button>
              </p>
            )}
            {o.path && !o.needsApproval && (
              <p className="row">
                <button onClick={() => check(o.id)}>{t("optimizers.check")}</button>
                {c && <span className={c.ok ? "muted" : "error-text"}>{c.ok ? (c.version ? t("optimizers.worksVersion", { version: c.version }) : t("optimizers.works")) : c.reason}</span>}
              </p>
            )}
          </div>
        );
      })}
      <p className="muted">{tNodes("optimizers.addAnother", { folder: <code>{"optimizers"}</code>, doc: <code>{"docs/DEVELOPING.md"}</code> })}</p>
    </section>
  );
}
