import { useEffect, useEffectEvent, useState, type FormEvent } from "react";
import { EngineService, type Profile, type ProjectInfo } from "./api.ts";
import { DownIcon, UpIcon } from "./icons.tsx";
import { basename } from "./paths.ts";
import { canMove, queueSummary, TASK_STATE_LABEL, type QueueTask } from "./queue.ts";
import { t } from "./i18n.ts";

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
  const failed = useEffectEvent((err: unknown) => onError(err));
  useEffect(() => {
    const load = () => EngineService.Queue().then(take).catch(failed);
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
      <h2>{t("app.queue")}</h2>
      <p className="muted">{t("queue.intro")}</p>
      <div className="row">
        <span role="status">{queueSummary(queue.tasks, queue.paused, limit)}</span>
        <button onClick={() => EngineService.SetQueue(!queue.paused, queue.limit).then(take).catch(onError)}>{queue.paused ? t("queue.resume") : t("queue.pause")}</button>
        <label className="row">
          {t("queue.atOnce")}
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
        <h3>{t("queue.addTitle")}</h3>
        <textarea aria-label={t("queue.task")} placeholder={t("queue.taskPlaceholder")} value={prompt} onChange={(e) => setPrompt(e.target.value)} rows={3} />
        <div className="row">
          <select aria-label={t("queue.project")} value={root} onChange={(e) => setRoot(e.target.value)}>
            {projects.map((p) => (
              <option key={p.id} value={p.root}>
                {p.name}
              </option>
            ))}
          </select>
          <select aria-label={t("queue.profile")} value={profileId} onChange={(e) => setProfileId(e.target.value)}>
            <option value="">{t("queue.defaultProfile")}</option>
            {profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <label className="row">
            <input type="checkbox" checked={worktree} onChange={(e) => setWorktree(e.target.checked)} /> {t("queue.ownWorktree")}
          </label>
          <button type="submit" className="primary" disabled={!prompt.trim() || !root}>
            {t("queue.add")}
          </button>
        </div>
      </form>
      {queue.tasks.length === 0 ? (
        <p className="muted">{t("queue.empty")}</p>
      ) : (
        <table className="usage-table queue-table">
          <thead>
            <tr>
              <th scope="col">{t("queue.task")}</th>
              <th scope="col">{t("queue.project")}</th>
              <th scope="col">{t("queue.state")}</th>
              <th scope="col">
                <span className="sr-only">{t("queue.actions")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {queue.tasks.map((task, i) => (
              <tr key={task.id}>
                <th scope="row">
                  {task.sessionId ? (
                    <button className="link" onClick={() => onOpen(task.sessionId!)}>
                      {task.title}
                    </button>
                  ) : (
                    <>{task.title}</>
                  )}
                  {task.note && <div className="muted">{task.note}</div>}
                </th>
                <td>{basename(task.projectRoot)}</td>
                <td>{TASK_STATE_LABEL[task.state] ? t(TASK_STATE_LABEL[task.state]) : task.state}</td>
                <td className="row">
                  <button className="icon-button" title={t("queue.moveUp")} aria-label={t("queue.moveUpNamed", { title: task.title })} disabled={!canMove(queue.tasks, i, -1)} onClick={() => EngineService.MoveTask(task.id, -1).then(take).catch(onError)}>
                    <UpIcon />
                  </button>
                  <button className="icon-button" title={t("queue.moveDown")} aria-label={t("queue.moveDownNamed", { title: task.title })} disabled={!canMove(queue.tasks, i, 1)} onClick={() => EngineService.MoveTask(task.id, 1).then(take).catch(onError)}>
                    <DownIcon />
                  </button>
                  <button aria-label={t("queue.removeNamed", { title: task.title })} onClick={() => EngineService.RemoveTask(task.id).then(take).catch(onError)}>
                    {t("common.remove")}
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
