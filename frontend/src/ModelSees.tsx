import { useState } from "react";
import { latestByPurpose, PURPOSE_LABEL, sections, sizeText, type ModelContext } from "./modelContext.ts";

// What the model sees besides the conversation (NFR-PRIV-003), from what the session
// recorded. All of it is plain text: the project's instructions come from the
// repository, and the page can call the engine.
export default function ModelSees({ records }: { records: readonly ModelContext[] }) {
  const kinds = latestByPurpose(records);
  return (
    <section className="model-sees" aria-labelledby="model-sees-title">
      <h3 id="model-sees-title">What the model sees</h3>
      {kinds.length === 0 ? (
        <p className="muted">
          Nothing recorded yet. A built-in session records this at its first model call. An agent that builds its own requests, as an external agent does,
          records nothing here: Freeloop sees only the messages and tool calls in this conversation.
        </p>
      ) : (
        <>
          <p className="muted">Besides the conversation, each model call carries this system prompt and these tools. It is recorded in the transcript whenever it changes.</p>
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
    navigator.clipboard
      .writeText(context.system)
      .then(() => {
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      })
      .catch(() => {});
  return (
    <details className="model-sees-kind" open={open}>
      <summary>
        {PURPOSE_LABEL[context.purpose] ?? context.purpose}
        <span className="muted">
          {" "}
          · {context.model}
          {context.effort ? `, effort ${context.effort}` : ""}
          {changes > 1 ? ` · changed ${changes - 1} ${changes === 2 ? "time" : "times"}` : ""}
        </span>
      </summary>
      {context.sent && <p className="muted">Also sent: {context.sent}.</p>}
      <div className="row model-sees-head">
        <span className="muted">System prompt, {sizeText(context.system.length)}</span>
        <button type="button" className="link" onClick={copy}>
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      {parts.map((p, i) => (
        <details key={i} className="model-sees-part">
          <summary>
            {p.title}
            {p.fromRepository && <span className="badge model-sees-repo">from the repository</span>}
            <span className="muted"> · {sizeText(p.chars)}</span>
          </summary>
          <pre>{p.text}</pre>
        </details>
      ))}
      {context.tools.length > 0 && (
        <>
          <p className="muted model-sees-head">{context.tools.length === 1 ? "1 tool" : `${context.tools.length} tools`}</p>
          <ul className="panel-list model-sees-tools">
            {context.tools.map((t) => (
              <li key={t.name}>
                <code>{t.name}</code>
                <div className="muted">{t.description}</div>
                {t.inputSchema != null && (
                  <details>
                    <summary>Input schema</summary>
                    <pre>{JSON.stringify(t.inputSchema, null, 2)}</pre>
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
