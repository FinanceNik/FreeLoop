// The phone app's calls to the phone server (GUI-052, internal/remote). Posts carry
// the app's header, which a page elsewhere cannot send, and the device's cookie.
import type { TranscriptEvent } from "../transcript.ts";
import type { PhoneOption } from "./phone.ts";

export class Unpaired extends Error {}

export interface PhoneSession {
  id: string;
  title: string;
  status: string;
  projectName: string;
  updated: string;
}

export interface PhoneRequest {
  requestId: string;
  sessionId: string;
  toolCallId: string;
  tool: string;
  title?: string;
  command?: string;
  paths?: string[];
  leavesSandbox?: boolean;
  options: PhoneOption[];
  onLaptop: boolean;
}

export interface PhoneAttention {
  sessionId: string;
  requests: PhoneRequest[];
  error?: string;
  stuck?: string;
}

export interface Summary {
  sessions: PhoneSession[];
  attention: PhoneAttention[];
}

async function call<T>(method: "GET" | "POST", path: string, body?: unknown): Promise<T> {
  const r = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: method === "POST" ? { "Content-Type": "application/json", "X-Freeloop-Phone": "1" } : {},
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (r.status === 401) throw new Unpaired();
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error((j as { error?: string }).error ?? r.statusText);
  return j as T;
}

export const api = {
  pair: (code: string, name: string) => call<{ id: string }>("POST", "/api/pair", { code, name }),
  pairAnswer: (id: string) => call<{ answer: "waiting" | "allowed" | "refused" }>("GET", `/api/pair/${encodeURIComponent(id)}`),
  summary: () => call<Summary>("GET", "/api/summary"),
  prompt: (sid: string, text: string) => call<{ queued: boolean }>("POST", `/api/sessions/${encodeURIComponent(sid)}/prompt`, { text }),
  cancel: (sid: string) => call("POST", `/api/sessions/${encodeURIComponent(sid)}/cancel`, {}),
  decide: (requestId: string, optionId: string) => call("POST", "/api/decide", { requestId, optionId }),
  logout: () => call("POST", "/api/logout", {}),
};

// followSummary calls back with every summary the server sends; the browser
// reconnects by itself, and each connection starts with a whole summary.
export function followSummary(on: (s: Summary) => void, onLost: () => void): () => void {
  const es = new EventSource("/api/stream");
  es.addEventListener("summary", (e) => on(JSON.parse((e as MessageEvent).data)));
  es.onerror = () => onLost();
  return () => es.close();
}

// followSession calls back with a session's events from after; on a reconnect the
// browser sends the last event's ID, and the server goes on from there.
export function followSession(sid: string, after: number, on: (ev: TranscriptEvent) => void, onLost: () => void): () => void {
  const es = new EventSource(`/api/sessions/${encodeURIComponent(sid)}/stream?after=${after}`);
  es.addEventListener("event", (e) => on(JSON.parse((e as MessageEvent).data)));
  es.onerror = () => onLost();
  return () => es.close();
}
