import { useEffect, useMemo, useState } from "react";
import { formAnswers, formFields, initialValues, type FormField, type FormValues } from "./forms.ts";

// An MCP server's request for input (MCP-008), answered on the tool card in
// the conversation or in the attention view: the same fields and checks in
// both.

export interface McpFormRequest {
  server: string;
  message: string;
  schema?: unknown;
  url?: string;
}

export interface McpFormState {
  fields: FormField[];
  values: FormValues;
  // An update builds on the latest answers, so two changes in a row both count.
  setValues: (v: FormValues | ((old: FormValues) => FormValues)) => void;
  error: string;
  // choose answers the request with an option: the allow option sends the
  // answers once they are complete, any other is sent as it is. Without a
  // form, every option is sent as it is.
  choose: (optionId: string, kind: string, decide: (optionId: string, answers?: Record<string, unknown>) => void) => void;
}

export function useMcpForm(form: McpFormRequest | null | undefined, requestId: string | undefined): McpFormState {
  const fields = useMemo(() => formFields(form?.schema), [form]);
  const [values, setValues] = useState<FormValues>({});
  const [error, setError] = useState("");
  useEffect(() => {
    setValues(initialValues(form?.schema));
    setError("");
  }, [requestId]);
  return {
    fields,
    values,
    setValues: (v) => {
      setValues(v);
      setError("");
    },
    error,
    choose: (optionId, kind, decide) => {
      if (!form || !kind.startsWith("allow")) return decide(optionId);
      const { answers, missing, invalid } = formAnswers(fields, values);
      if (missing.length || invalid.length) {
        setError([missing.length ? `Fill in ${missing.join(", ")}.` : "", invalid.length ? `Check ${invalid.join(", ")}.` : ""].join(" ").trim());
        return;
      }
      decide(optionId, answers);
    },
  };
}

// Up to this many choices show as a list to pick from, more as a menu.
const CHOICES_SHOWN = 8;

// McpFormView shows the request: its message as plain text, one field per
// property, and a link it asks the user to open, shown in full so the user
// sees where it goes.
export function McpFormView({ form, state, idPrefix = "form" }: { form: McpFormRequest; state: McpFormState; idPrefix?: string }) {
  const { fields, values, setValues, error } = state;
  const set = (name: string, v: string | boolean) => setValues((old) => ({ ...old, [name]: v }));
  return (
    <div className="mcp-form">
      <p>
        <strong>{form.server}</strong> asks: {form.message}
      </p>
      {form.url && (
        <p>
          Open <code>{form.url}</code> to continue, then choose Send.
        </p>
      )}
      {fields.map((f) => {
        const id = `${idPrefix}-${f.name}`;
        const label = (
          <label htmlFor={id}>
            {f.label}
            {f.required && <span aria-hidden="true"> *</span>}
          </label>
        );
        if (f.type === "boolean") {
          return (
            <div key={f.name} className="mcp-field mcp-field-check">
              <input id={id} type="checkbox" checked={values[f.name] === true} onChange={(e) => set(f.name, e.target.checked)} />
              {label}
              {f.description && <span className="muted"> {f.description}</span>}
            </div>
          );
        }
        const value = typeof values[f.name] === "string" ? (values[f.name] as string) : "";
        // A short list of choices reads as a list, with what each means; a long one stays
        // a menu (HRN-021).
        if ((f.type === "enum" && f.options!.length <= CHOICES_SHOWN) || f.type === "multi") {
          const picked = Array.isArray(values[f.name]) ? (values[f.name] as string[]) : [];
          return (
            <fieldset key={f.name} className="mcp-field mcp-choices">
              <legend>
                {f.label}
                {f.required && <span aria-hidden="true"> *</span>}
              </legend>
              {f.description && <span className="muted">{f.description}</span>}
              {f.options!.map((o, i) => (
                <label key={o.value} className="mcp-choice">
                  {f.type === "multi" ? (
                    <input type="checkbox" checked={picked.includes(o.value)} onChange={(e) => {
                        const on = e.target.checked;
                        setValues((old) => {
                          const was = Array.isArray(old[f.name]) ? (old[f.name] as string[]) : [];
                          return { ...old, [f.name]: on ? [...was.filter((x) => x !== o.value), o.value] : was.filter((x) => x !== o.value) };
                        });
                      }} />
                  ) : (
                    <input type="radio" name={id} id={i === 0 ? id : undefined} checked={value === o.value} onChange={() => set(f.name, o.value)} />
                  )}
                  <span>
                    {o.label}
                    {o.description && <span className="muted mcp-choice-detail">{o.description}</span>}
                  </span>
                </label>
              ))}
            </fieldset>
          );
        }
        return (
          <div key={f.name} className="mcp-field">
            {label}
            {f.type === "enum" ? (
              <select id={id} value={value} required={f.required} onChange={(e) => set(f.name, e.target.value)}>
                <option value="">Choose…</option>
                {f.options!.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>
            ) : (
              <input
                id={id}
                type={f.type === "string" ? (f.format === "email" ? "email" : f.format === "date" ? "date" : "text") : "number"}
                step={f.type === "integer" ? 1 : "any"}
                required={f.required}
                value={value}
                onChange={(e) => set(f.name, e.target.value)}
              />
            )}
            {f.description && <span className="muted">{f.description}</span>}
          </div>
        );
      })}
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
