// Forms for an MCP server's request for input (MCP-008) and an agent's questions
// (HRN-021). The asker sends a flat JSON Schema object of strings, numbers, booleans,
// enums and lists of enum values to choose several of; the card shows one field per
// property and sends the answers back.

export interface FormField {
  name: string;
  label: string;
  description?: string;
  type: "string" | "number" | "integer" | "boolean" | "enum" | "multi";
  options?: { value: string; label: string; description?: string }[];
  required: boolean;
  format?: string;
}

export type FormValues = Record<string, string | boolean | string[]>;

type Prop = {
  type?: string;
  title?: string;
  description?: string;
  enum?: unknown[];
  enumNames?: unknown[];
  oneOf?: Choice[];
  items?: { enum?: unknown[]; anyOf?: Choice[]; oneOf?: Choice[] };
  format?: string;
  default?: unknown;
};

type Choice = { const?: unknown; title?: string; description?: string };

// choices reads titled enum options, with what each means when given.
function choices(list: unknown): FormField["options"] | null {
  if (!Array.isArray(list) || !list.every((o) => o && typeof o === "object" && "const" in o)) return null;
  return (list as Choice[]).map((o) => ({ value: String(o.const), label: o.title || String(o.const), ...(o.description ? { description: String(o.description) } : {}) }));
}

// formFields reads the fields from a schema; properties of a kind the
// specification does not allow (objects, arrays) are left out.
export function formFields(schema: unknown): FormField[] {
  const s = schema as { properties?: Record<string, Prop>; required?: unknown[] } | null;
  if (!s || typeof s !== "object" || !s.properties || typeof s.properties !== "object") return [];
  const required = new Set(Array.isArray(s.required) ? s.required.map(String) : []);
  const out: FormField[] = [];
  for (const [name, p] of Object.entries(s.properties)) {
    if (!p || typeof p !== "object") continue;
    const base = { name, label: p.title || name, description: p.description, required: required.has(name) };
    if (Array.isArray(p.enum)) {
      const names = Array.isArray(p.enumNames) ? p.enumNames : [];
      out.push({ ...base, type: "enum", options: p.enum.map((v, i) => ({ value: String(v), label: String(names[i] ?? v) })) });
    } else if (choices(p.oneOf)) {
      out.push({ ...base, type: "enum", options: choices(p.oneOf)! });
    } else if (p.type === "array" && p.items) {
      const listed = Array.isArray(p.items.enum) ? p.items.enum.map((v) => ({ value: String(v), label: String(v) })) : (choices(p.items.anyOf) ?? choices(p.items.oneOf));
      if (listed) out.push({ ...base, type: "multi", options: listed });
    } else if (p.type === "string" || p.type === "number" || p.type === "integer" || p.type === "boolean") {
      out.push({ ...base, type: p.type, format: p.format });
    }
  }
  return out;
}

// initialValues fills each field with the schema's default, if any.
export function initialValues(schema: unknown): FormValues {
  const props = (schema as { properties?: Record<string, Prop> } | null)?.properties ?? {};
  const out: FormValues = {};
  for (const f of formFields(schema)) {
    const d = props[f.name]?.default;
    if (f.type === "boolean") out[f.name] = d === true;
    else if (f.type === "multi") out[f.name] = Array.isArray(d) ? d.map(String) : [];
    else if (d !== undefined && d !== null) out[f.name] = String(d);
  }
  return out;
}

// formAnswers turns what the user entered into the answers to send: numbers
// as numbers, empty optional fields left out. missing names the required
// fields still empty, and invalid those that are not numbers.
export function formAnswers(
  fields: FormField[],
  values: FormValues,
): { answers: Record<string, unknown>; missing: string[]; invalid: string[] } {
  const answers: Record<string, unknown> = {};
  const missing: string[] = [];
  const invalid: string[] = [];
  for (const f of fields) {
    const v = values[f.name];
    if (f.type === "boolean") {
      answers[f.name] = v === true;
      continue;
    }
    if (f.type === "multi") {
      const picked = (Array.isArray(v) ? v : []).filter((x) => f.options?.some((o) => o.value === x));
      if (picked.length) answers[f.name] = picked;
      else if (f.required) missing.push(f.label);
      continue;
    }
    const text = typeof v === "string" ? v.trim() : "";
    if (text === "") {
      if (f.required) missing.push(f.label);
      continue;
    }
    if (f.type === "number" || f.type === "integer") {
      const n = Number(text);
      if (!Number.isFinite(n) || (f.type === "integer" && !Number.isInteger(n))) invalid.push(f.label);
      else answers[f.name] = n;
      continue;
    }
    if (f.type === "enum" && f.options && !f.options.some((o) => o.value === text)) {
      invalid.push(f.label);
      continue;
    }
    answers[f.name] = f.type === "string" ? (typeof v === "string" ? v : "") : text;
  }
  return { answers, missing, invalid };
}

// agentQuestions reads the questions an agent's call carries (HRN-021): Claude Code's
// AskUserQuestion and the built-in agent's ask_user, so the card shows them as
// questions, not as the call's input.
export function agentQuestions(input: unknown): { question: string; options: string[] }[] | null {
  const list = (input as { questions?: unknown } | null)?.questions;
  if (!Array.isArray(list) || list.length === 0) return null;
  const out = list
    .filter((q): q is { question: string; options?: unknown } => !!q && typeof q === "object" && typeof (q as { question?: unknown }).question === "string")
    .map((q) => ({
      question: q.question,
      options: Array.isArray(q.options) ? q.options.map((o) => (o && typeof o === "object" ? String((o as { label?: unknown }).label ?? "") : String(o))).filter(Boolean) : [],
    }));
  return out.length ? out : null;
}

