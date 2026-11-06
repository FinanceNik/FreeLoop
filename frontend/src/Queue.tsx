import { useEffect, useState, type FormEvent } from "react";
import { EngineService, type Profile, type ProjectInfo } from "./api.ts";
import { basename } from "./paths.ts";
import { canMove, queueSummary, TASK_STATE_LABEL, type QueueTask } from "./queue.ts";

type QueueState = { paused: boolean; limit: number; tasks: QueueTask[] };

// Queue lists tasks that start as sessions when there is room (SES-025): under the
// queue's limit, and with the task's agent not at a limit warning. A started task is
// an ordinary session, tagged queued; every tool call there asks as usual.
export default function Queue({ projects, profiles, onOpen, onError }: { projects: ProjectInfo[]; profiles: Profile[]; onOpen: (sid: string) => void; onError: (err: unknown) => void }) {
  const [queue, setQueue] = useState<QueueState | null>(null);
  const [prompt, setPrompt] = useState("");
  const [chosenRoot, setRoot] = useState("");
  // The list of projects can arrive after the page.
  const root = chosenRoot || projects[0]?.root || "";
  const [profileId, setProfileId] = useState("");
  const [worktree, setWorktree] = useState(true);
  const take = (q: unknown) => setQueue(q as QueueState);
  useEffect(() => {
    const load = () => EngineService.Queue().then(take).catch(onError);
    load();
    // The queue starts tasks every 30 seconds; this follows it.
    const every = setInterval(load, 3000);
    return () => clearInterval(every);
  }, []);
  if (!queue) return null;
  const limit = queue.limit > 0 ? queue.limit : 3;
  const add = (e: FormEvent) => {
    e.preventDefault();
    // Only what a new task says: the engine fills in the rest, its times included.
    EngineService.AddTask({ prompt, projectRoot: root, profileId, worktree } as never)
      .then((q) => {
        take(q);
        setPrompt("");
      })
      .catch(onError);
  };
  return (
    <section className="page">
      <h2>Queue</h2>
      <p className="muted">
        Tasks start as sessions when there is room: under the limit below, and with their agent not near its account limit, whose reset they then wait
        for. Each runs as an ordinary session, and asks for every tool call as usual.
      </p>
      <div className="row">
        <span role="status">{queueSummary(queue.tasks, queue.paused, limit)}</span>
        <button onClick={() => EngineService.SetQueue(!queue.paused, queue.limit).then(take).catch(onError)}>{queue.paused ? "Resume" : "Pause"}</button>
        <label className="row">
          At once
          <input
            type="number"
            min={1}
            max={20}
            value={limit}
            onChange={(e) => EngineService.SetQueue(queue.paused, Number(e.target.value)).then(take).catch(onError)}
            style={{ width: "4em" }}
          />
        </label>
      </div>
      <form className="card" onSubmit={add}>
        <h3>Add a task</h3>
        <textarea aria-label="Task" placeholder="What the agent should do" value={prompt} onChange={(e) => setPrompt(e.target.value)} rows={3} />
        <div className="row">
          <select aria-label="Project" value={root} onChange={(e) => setRoot(e.target.value)}>
            {projects.map((p) => (
              <option key={p.id} value={p.root}>
                {p.name}
              </option>
            ))}
          </select>
          <select aria-label="Profile" value={profileId} onChange={(e) => setProfileId(e.target.value)}>
            <option value="">Project's default profile</option>
            {profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <label className="row">
            <input type="checkbox" checked={worktree} onChange={(e) => setWorktree(e.target.checked)} /> Its own worktree
          </label>
          <button type="submit" className="primary" disabled={!prompt.trim() || !root}>
            Add to queue
          </button>
        </div>
      </form>
      {queue.tasks.length === 0 ? (
        <p className="muted">No tasks yet.</p>
      ) : (
        <table className="usage-table queue-table">
          <thead>
            <tr>
              <th scope="col">Task</th>
              <th scope="col">Project</th>
              <th scope="col">State</th>
              <th scope="col">
                <span className="sr-only">Actions</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {queue.tasks.map((t, i) => (
              <tr key={t.id}>
                <th scope="row">
                  {t.sessionId ? (
                    <button className="link" onClick={() => onOpen(t.sessionId!)}>
                      {t.title}
                    </button>
                  ) : (
                    t.title
                  )}
                  {t.note && <div className="muted">{t.note}</div>}
                </th>
                <td>{basename(t.projectRoot)}</td>
                <td>{TASK_STATE_LABEL[t.state] ?? t.state}</td>
                <td className="row">
                  <button aria-label={`Move ${t.title} up`} disabled={!canMove(queue.tasks, i, -1)} onClick={() => EngineService.MoveTask(t.id, -1).then(take).catch(onError)}>
                    ↑
                  </button>
                  <button aria-label={`Move ${t.title} down`} disabled={!canMove(queue.tasks, i, 1)} onClick={() => EngineService.MoveTask(t.id, 1).then(take).catch(onError)}>
                    ↓
                  </button>
                  <button aria-label={`Remove ${t.title}`} onClick={() => EngineService.RemoveTask(t.id).then(take).catch(onError)}>
                    Remove
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
