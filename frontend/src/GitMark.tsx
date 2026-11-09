import { GitBranchIcon } from "./icons.tsx";
import { t } from "./i18n.ts";

// GitMark says a session works in a git repository, and on which branch, in its
// tooltip and accessible name (GUI-044).
export default function GitMark({ branch }: { branch?: string }) {
  const name = branch ? t("app.git.onBranch", { branch }) : t("app.git.repo");
  return (
    <span className="git-mark" role="img" aria-label={name} title={name}>
      <GitBranchIcon />
    </span>
  );
}
