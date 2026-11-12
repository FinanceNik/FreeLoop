import { Browser } from "@wailsio/runtime";
import { useEffect, useEffectEvent, useState, type FormEvent } from "react";
import { EngineService, type RegistryOption, type RegistryServer } from "./api.ts";
import { list, t } from "./i18n.ts";
import { tNodes } from "./i18nNodes.tsx";

// McpRegistry browses a server registry (MCP-009). Nothing connects until the
// user sets its address. Choosing a result only fills in the add-server form:
// anyone can publish to a registry, so the server still starts only after the
// user confirms its exact command.

export const OFFICIAL_REGISTRY = "https://registry.modelcontextprotocol.io";

interface Props {
  onPick: (server: RegistryServer, option: RegistryOption) => void;
  onClose: () => void;
  onError: (err: unknown) => void;
}

export default function McpRegistry({ onPick, onClose, onError }: Props) {
  const [address, setAddress] = useState<string | null>(null);
  const [typed, setTyped] = useState("");
  const [query, setQuery] = useState("");
  const [servers, setServers] = useState<RegistryServer[] | null>(null);
  const [next, setNext] = useState("");
  const [busy, setBusy] = useState(false);

  const failed = useEffectEvent((err: unknown) => onError(err));
  useEffect(() => {
    EngineService.MCPRegistryAddress()
      .then((a) => setAddress(a))
      .catch(failed);
  }, []);

  const search = (cursor = "") => {
    setBusy(true);
    EngineService.SearchMCPRegistry(query, cursor)
      .then((page) => {
        setServers(cursor ? [...(servers ?? []), ...(page.servers ?? [])] : (page.servers ?? []));
        setNext(page.next);
      })
      .catch(onError)
      .finally(() => setBusy(false));
  };
  const use = (url: string) =>
    EngineService.SetMCPRegistryAddress(url)
      .then(() => setAddress(url))
      .catch(onError);

  if (address === null) return null;
  if (!address) {
    return (
      <section className="page">
        <h2>{t("mcp.registry.title")}</h2>
        <div className="card form">
          <p>{t("mcp.registry.privacy")}</p>
          <div className="row">
            <button className="primary" onClick={() => use(OFFICIAL_REGISTRY)}>
              {t("mcp.registry.useOfficial")}
            </button>
            <button onClick={onClose}>{t("common.back")}</button>
          </div>
          <p className="muted">{tNodes("mcp.registry.wouldConnect", { url: <code>{OFFICIAL_REGISTRY}</code> })}</p>
          <label>
            {t("mcp.registry.other")}
            <input value={typed} onChange={(e) => setTyped(e.target.value)} placeholder="https://registry.example.com" />
          </label>
          <button disabled={!typed.trim()} onClick={() => use(typed.trim())}>
            {t("mcp.registry.useAddress")}
          </button>
        </div>
      </section>
    );
  }
  return (
    <section className="page">
      <h2>{t("mcp.registry.title")}</h2>
      <p className="muted">
        {tNodes("mcp.registry.from", { address: <code>{address}</code> })}{" "}
        <button type="button" className="link" onClick={() => use("")}>
          {t("mcp.registry.change")}
        </button>
      </p>
      <form
        className="row"
        role="search"
        onSubmit={(e: FormEvent) => {
          e.preventDefault();
          search();
        }}
      >
        <input aria-label={t("mcp.registry.searchLabel")} value={query} onChange={(e) => setQuery(e.target.value)} placeholder={t("mcp.registry.searchPlaceholder")} />
        <button className="primary" disabled={busy}>
          {t("mcp.registry.search")}
        </button>
        <button type="button" onClick={onClose}>
          {t("common.back")}
        </button>
      </form>
      {servers?.length === 0 && <p className="muted">{t("mcp.registry.nothing")}</p>}
      <ul className="provider-list registry-list" aria-label={t("mcp.registry.servers")}>
        {(servers ?? []).map((s) => (
          <li key={s.name} className="card">
            <div>
              <strong>{s.title || s.name}</strong> <span className="muted">{s.version}</span>
              <div className="muted">{s.name}</div>
              <p>{s.description}</p>
              {s.repository && (
                <button type="button" className="link" onClick={() => Browser.OpenURL(s.repository).catch(onError)}>
                  {t("mcp.registry.repository")}
                </button>
              )}
              {(s.unsupported ?? []).length > 0 && <div className="muted">{t("mcp.registry.unsupported", { list: list(s.unsupported ?? []) })}</div>}
            </div>
            <div className="row wrap">
              {(s.options ?? []).map((o) => (
                <button key={o.label} onClick={() => onPick(s, o)} aria-label={t("mcp.registry.addLabel", { name: s.name, option: o.label })}>
                  {t("mcp.registry.addOption", { option: o.label })}
                </button>
              ))}
              {(s.options ?? []).length === 0 && <span className="muted">{t("mcp.registry.noWay")}</span>}
            </div>
          </li>
        ))}
      </ul>
      {next && (
        <button disabled={busy} onClick={() => search(next)}>
          {t("mcp.registry.more")}
        </button>
      )}
    </section>
  );
}
