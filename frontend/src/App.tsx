import { useEffect, useRef, useState, type FormEvent } from "react";
import { Events } from "@wailsio/runtime";
import { EngineService } from "../bindings/github.com/FinanceNik/Freeloop/cmd/freeloop";
import type { Issue } from "../bindings/github.com/FinanceNik/Freeloop/internal/config/models";
import { Type as EventType } from "../bindings/github.com/FinanceNik/Freeloop/internal/event/models";
import type { Session } from "../bindings/github.com/FinanceNik/Freeloop/internal/session/models";
import { apply, emptyTranscript, summary, type Transcript, type TranscriptEvent } from "./transcript.ts";

// Must match EventName in cmd/freeloop/main.go.
const EVENT_NAME = "freeloop:event";

export default function App() {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [transcripts, setTranscripts] = useState<Record<string, Transcript>>({});
  const [issues, setIssues] = useState<Issue[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [draft, setDraft] = useState("");

  // The reducer state lives in a ref so event callbacks always see the latest
  // transcripts; React state mirrors it for rendering.
  const transcriptsRef = useRef<Record<string, Transcript>>({});
  // Per session: a catch-up fetch is in flight, and whether another gap was
  // seen meanwhile and needs one more fetch.
  const fetching = useRef<Map<string, boolean>>(new Map());

  const report = (err: unknown) => setError(err instanceof Error ? err.message : String(err));

  const refreshSessions = () =>
    EngineService.ListSessions()
      .then((list) => setSessions(list ?? []))
      .catch(report);

  const ingest = (sid: string, events: readonly TranscriptEvent[]) => {
    const current = transcriptsRef.current[sid] ?? emptyTranscript;
    const { transcript, missingAfter } = apply(current, events);
    if (transcript !== current) {
      transcriptsRef.current = { ...transcriptsRef.current, [sid]: transcript };
      setTranscripts(transcriptsRef.current);
    }
    if (missingAfter !== null) catchUp(sid);
  };

  const catchUp = (sid: string) => {
    if (fetching.current.has(sid)) {
      fetching.current.set(sid, true);
      return;
    }
    fetching.current.set(sid, false);
    const after = transcriptsRef.current[sid]?.lastSeq ?? 0;
    EngineService.Events(sid, after)
      .then((events) => {
        const again = fetching.current.get(sid);
        fetching.current.delete(sid);
        ingest(sid, events ?? []);
        if (again) catchUp(sid);
      })
      .catch((err) => {
        fetching.current.delete(sid);
        report(err);
      });
  };

  useEffect(() => {
    refreshSessions();
    EngineService.ConfigIssues()
      .then((list) => setIssues(list ?? []))
      .catch(report);

    return Events.On(EVENT_NAME, ({ data: ev }) => {
      ingest(ev.sid, [ev]);
      setSessions((list) => {
        const i = list.findIndex((s) => s.id === ev.sid);
        if (i < 0) {
          refreshSessions();
          return list;
        }
        const s = { ...list[i], lastSeq: Math.max(list[i].lastSeq, ev.seq), updatedAt: ev.ts };
        if (ev.type === EventType.SessionStatus) s.status = (ev.data as { status: Session["status"] }).status;
        return [s, ...list.slice(0, i), ...list.slice(i + 1)];
      });
    });
  }, []);

  const select = (sid: string) => {
    setSelectedId(sid);
    if (!transcriptsRef.current[sid]) catchUp(sid);
  };

  const newSession = () =>
    EngineService.CreateSession({ title: "" })
      .then((s) => {
        setSessions((list) => (list.some((x) => x.id === s.id) ? list : [s, ...list]));
        select(s.id);
      })
      .catch(report);

  const appendTestEvent = (e: FormEvent) => {
    e.preventDefault();
    if (!selectedId || !draft.trim()) return;
    EngineService.AppendEvent({
      sessionId: selectedId,
      type: EventType.UserMessageChunk,
      data: { content: { type: "text", text: draft } },
    })
      .then((ev) => {
        setDraft("");
        ingest(ev.sid, [ev]);
      })
      .catch(report);
  };

  const selected = sessions.find((s) => s.id === selectedId) ?? null;
  const events = selectedId ? (transcripts[selectedId]?.events ?? []) : [];

  return (
    <div className="app">
      <aside className="sidebar">
        <header>
          <h1>Freeloop</h1>
          <button onClick={newSession}>New session</button>
        </header>
        <nav aria-label="Sessions">
          <ul>
            {sessions.map((s) => (
              <li key={s.id}>
                <button aria-current={s.id === selectedId ? "true" : undefined} onClick={() => select(s.id)}>
                  <span>{s.title}</span>
                  <small>
                    {s.status} · {s.lastSeq} events
                  </small>
                </button>
              </li>
            ))}
          </ul>
          {sessions.length === 0 && <p>No sessions yet.</p>}
        </nav>
      </aside>

      <main className="main">
        {issues.length > 0 && (
          <div role="alert">
            <strong>Configuration problems; using defaults where needed.</strong>
            <ul>
              {issues.map((i, n) => (
                <li key={n}>
                  {i.file}
                  {i.field ? ` (${i.field})` : ""}: {i.message}
                </li>
              ))}
            </ul>
          </div>
        )}
        {error && (
          <div role="alert">
            {error} <button onClick={() => setError(null)}>Dismiss</button>
          </div>
        )}

        {selected ? (
          <>
            <header>
              <h2>{selected.title}</h2>
              <p>
                <code>{selected.id}</code> · {selected.harnessId}
              </p>
            </header>
            <ol className="events">
              {events.map((ev) => (
                <li key={ev.seq}>
                  <code>
                    #{ev.seq} {ev.type}
                  </code>{" "}
                  {summary(ev)}
                </li>
              ))}
            </ol>
            <form onSubmit={appendTestEvent}>
              <input
                aria-label="Message text"
                placeholder="Text for a test user_message_chunk event"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
              />
              <button type="submit" disabled={!draft.trim()}>
                Append event
              </button>
            </form>
          </>
        ) : (
          <p>Select a session or create a new one.</p>
        )}
      </main>
    </div>
  );
}
