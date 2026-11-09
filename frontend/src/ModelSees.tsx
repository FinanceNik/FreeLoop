import { useState } from "react";
import { latestByPurpose, PURPOSE_LABEL, sections, sizeText, type ModelContext } from "./modelContext.ts";
import { t } from "./i18n.ts";

// What the model sees besides the conversation (NFR-PRIV-003), from what the session
// recorded. All of it is plain text: the project's instructions come from the
// repository, and the page can call the engine.
export default function ModelSees({ records }: { records: readonly ModelContext[] }) {
  const kinds = latestByPurpose(records);
  return (
    <section className="model-sees" aria-labelledby="model-sees-title">
      <h3 id="model-sees-title">{t("modelSees.title")}</h3>
      {kinds.length === 0 ? (
        <p className="muted">{t("modelSees.none")}</p>
      ) : (
        <>
          <p className="muted">{t("modelSees.intro")}</p>
          {kinds.map(({ context, changes }) => (
            <Kind key={context.purpose} context={context} changes={changes} open={context.purpose === "turn"} />
          ))}
        </>
      )}
    </section>
  );
}

function Kind({ context, changes, open }: { context: ModelContext; changes: number; open: boolean }) {
  const [copied, setCopied] = useState(false);
  const parts = sections(context.system);
  const copy = () =>
    navigator.clipboard.writeText(context.system).then(
      () => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      },
      () => {},
    );
  return (
    <details className="model-sees-kind" open={open}>
      <summary>
        {PURPOSE_LABEL[context.purpose] ? t(PURPOSE_LABEL[context.purpose]) : context.purpose}
        <span className="muted">
          {" "}
          · {context.effort ? t("modelSees.effort", { model: context.model, effort: context.effort }) : context.model}
          {changes > 1 && " · "}
          {changes > 1 && t("modelSees.changed", { count: changes - 1 })}
        </span>
      </summary>
      {context.sent && <p className="muted">{t("modelSees.sent", { sent: context.sent })}</p>}
      <div className="row model-sees-head">
        <span className="muted">{t("modelSees.systemPrompt", { size: sizeText(context.system.length) })}</span>
        <button type="button" className="link" onClick={copy}>
          {copied ? t("modelSees.copied") : t("modelSees.copy")}
        </button>
      </div>
      {parts.map((p, i) => (
        <details key={i} className="model-sees-part">
          <summary>
            {p.title}
            {p.fromRepository && <span className="badge model-sees-repo">{t("modelSees.fromRepository")}</span>}
            <span className="muted"> · {sizeText(p.chars)}</span>
          </summary>
          <pre>{p.text}</pre>
        </details>
      ))}
      {context.tools.length > 0 && (
        <>
          <p className="muted model-sees-head">{t("modelSees.tools", { count: context.tools.length })}</p>
          <ul className="panel-list model-sees-tools">
            {context.tools.map((tool) => (
              <li key={tool.name}>
                <code>{tool.name}</code>
                <div className="muted">{tool.description}</div>
                {tool.inputSchema != null && (
                  <details>
                    <summary>{t("modelSees.inputSchema")}</summary>
                    <pre>{JSON.stringify(tool.inputSchema, null, 2)}</pre>
                  </details>
                )}
              </li>
            ))}
          </ul>
        </>
      )}
    </details>
  );
}
