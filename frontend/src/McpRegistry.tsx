import { Browser } from "@wailsio/runtime";
import { useEffect, useState, type FormEvent } from "react";
import { EngineService, type RegistryOption, type RegistryServer } from "./api.ts";

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

  useEffect(() => {
    EngineService.MCPRegistryAddress()
      .then((a) => setAddress(a))
      .catch(onError);
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
        <h2>Browse a server registry</h2>
        <div className="card form">
          <p>Freeloop connects only to addresses you set. Searching sends your search words to the registry you choose, and nothing else.</p>
          <div className="row">
            <button className="primary" onClick={() => use(OFFICIAL_REGISTRY)}>
              Use the official MCP registry
            </button>
            <button onClick={onClose}>Back</button>
          </div>
          <p className="muted">
            It would connect to <code>{OFFICIAL_REGISTRY}</code>.
          </p>
          <label>
            Or another registry's address
            <input value={typed} onChange={(e) => setTyped(e.target.value)} placeholder="https://registry.example.com" />
          </label>
          <button disabled={!typed.trim()} onClick={() => use(typed.trim())}>
            Use this address
          </button>
        </div>
      </section>
    );
  }
  return (
    <section className="page">
      <h2>Browse a server registry</h2>
      <p className="muted">
        From <code>{address}</code>. Anyone can publish there, so read what a server is before adding it. Adding only fills in the form; it starts after you confirm
        its exact command.{" "}
        <button type="button" className="link" onClick={() => use("")}>
          Change the address
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
        <input aria-label="Search the registry" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Search by name" />
        <button className="primary" disabled={busy}>
          Search
        </button>
        <button type="button" onClick={onClose}>
          Back
        </button>
      </form>
      {servers?.length === 0 && <p className="muted">Nothing matches.</p>}
      <ul className="provider-list registry-list" aria-label="Servers">
        {(servers ?? []).map((s) => (
          <li key={s.name} className="card">
            <div>
              <strong>{s.title || s.name}</strong> <span className="muted">{s.version}</span>
              <div className="muted">{s.name}</div>
              <p>{s.description}</p>
              {s.repository && (
                <button type="button" className="link" onClick={() => Browser.OpenURL(s.repository).catch(onError)}>
                  Its repository
                </button>
              )}
              {(s.unsupported ?? []).length > 0 && <div className="muted">Also offered, which Freeloop cannot run: {(s.unsupported ?? []).join(", ")}</div>}
            </div>
            <div className="row wrap">
              {(s.options ?? []).map((o) => (
                <button key={o.label} onClick={() => onPick(s, o)} aria-label={`Add ${s.name}, ${o.label}`}>
                  Add: {o.label}
                </button>
              ))}
              {(s.options ?? []).length === 0 && <span className="muted">No way to run it here</span>}
            </div>
          </li>
        ))}
      </ul>
      {next && (
        <button disabled={busy} onClick={() => search(next)}>
          More
        </button>
      )}
    </section>
  );
}
