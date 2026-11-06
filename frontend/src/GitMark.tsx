import { GitBranchIcon } from "./icons.tsx";

// GitMark says a session works in a git repository, and on which branch, in its
// tooltip and accessible name (GUI-044).
export default function GitMark({ branch }: { branch?: string }) {
  const name = branch ? `Git repository, on ${branch}` : "Git repository";
  return (
    <span className="git-mark" role="img" aria-label={name} title={name}>
      <GitBranchIcon />
    </span>
  );
}
