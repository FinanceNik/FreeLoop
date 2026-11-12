// The phone app's calls to the phone server (GUI-052, internal/remote). Posts carry
// the app's header, which a page elsewhere cannot send. The phone's key is kept in this
// page's own storage and sent as a header on every call, streams included, and never
// as a cookie, which the browser would send to every port of the same host (SEC-020).
import type { TranscriptEvent } from "../transcript.ts";
import { parseSSE, reconnectDelay, type PhoneOption, type SSEMessage } from "./phone.ts";

const KEY = "freeloop.phone.key";

function authHeaders(): Record<string, string> {
  const key = localStorage.getItem(KEY);
  return key ? { Authorization: `Bearer ${key}` } : {};
}

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
    credentials: "omit",
    headers: { ...authHeaders(), ...(method === "POST" ? { "Content-Type": "application/json", "X-Freeloop-Phone": "1" } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (r.status === 401) {
    localStorage.removeItem(KEY);
    throw new Unpaired();
  }
  const j = await r.json().catch(() => ({}));
  if (!r.ok) throw new Error((j as { error?: string }).error ?? r.statusText);
  return j as T;
}

export const api = {
  pair: (code: string, name: string) => call<{ id: string }>("POST", "/api/pair", { code, name }),
  // The key comes once, with the answer "allowed", and is kept here.
  pairAnswer: async (id: string) => {
    const r = await call<{ answer: "waiting" | "allowed" | "refused"; token?: string }>("GET", `/api/pair/${encodeURIComponent(id)}`);
    if (r.answer === "allowed" && r.token) localStorage.setItem(KEY, r.token);
    return { answer: r.answer };
  },
  summary: () => call<Summary>("GET", "/api/summary"),
  prompt: (sid: string, text: string) => call<{ queued: boolean }>("POST", `/api/sessions/${encodeURIComponent(sid)}/prompt`, { text }),
  cancel: (sid: string) => call("POST", `/api/sessions/${encodeURIComponent(sid)}/cancel`, {}),
  decide: (requestId: string, optionId: string) => call("POST", "/api/decide", { requestId, optionId }),
  pushKey: () => call<{ key: string }>("GET", "/api/push/key"),
  pushSubscribe: (body: { endpoint: string; keys: { p256dh: string; auth: string } }) => call("POST", "/api/push/subscribe", body),
  pushUnsubscribe: () => call("POST", "/api/push/unsubscribe", {}),
  logout: () => call("POST", "/api/logout", {}).then((r) => (localStorage.removeItem(KEY), r)),
};

// follow reads a server-sent stream with fetch, so the key goes as a header, and takes
// it up again after a pause when it drops, from the last message's ID. It tells onLost
// each time the connection ends or fails, and stops by itself on a refusal.
function follow(url: string, on: (m: SSEMessage) => void, onLost: () => void): () => void {
  const abort = new AbortController();
  void (async () => {
    let attempt = 0;
    let lastId = "";
    while (!abort.signal.aborted) {
      try {
        const r = await fetch(url, {
          credentials: "omit",
          signal: abort.signal,
          headers: { ...authHeaders(), ...(lastId ? { "Last-Event-ID": lastId } : {}) },
        });
        if (r.status === 401) {
          localStorage.removeItem(KEY);
          onLost();
          return;
        }
        if (!r.ok || !r.body) throw new Error(r.statusText);
        const reader = r.body.getReader();
        const decoder = new TextDecoder();
        let rest = "";
        for (;;) {
          const { done, value } = await reader.read();
          if (done) break;
          attempt = 0;
          const out = parseSSE(rest, decoder.decode(value, { stream: true }));
          rest = out.rest;
          for (const m of out.messages) {
            if (m.id) lastId = m.id;
            on(m);
          }
        }
      } catch {
        if (abort.signal.aborted) return;
      }
      if (abort.signal.aborted) return;
      onLost();
      await new Promise((resolve) => setTimeout(resolve, reconnectDelay(attempt++)));
    }
  })();
  return () => abort.abort();
}

// followSummary calls back with every summary the server sends; each connection
// starts with a whole summary.
export function followSummary(on: (s: Summary) => void, onLost: () => void): () => void {
  return follow(
    "/api/stream",
    (m) => {
      if (m.event === "summary") on(JSON.parse(m.data));
    },
    onLost,
  );
}

// followSession calls back with a session's events from after; on a reconnect it sends
// the last event's ID, and the server goes on from there.
export function followSession(sid: string, after: number, on: (ev: TranscriptEvent) => void, onLost: () => void): () => void {
  return follow(
    `/api/sessions/${encodeURIComponent(sid)}/stream?after=${after}`,
    (m) => {
      if (m.event === "event") on(JSON.parse(m.data));
    },
    onLost,
  );
}
