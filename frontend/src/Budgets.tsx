import { useEffect, useState } from "react";
import { EngineService, type BudgetState } from "./api.ts";
import { budgetLine, budgetName } from "./budgets.ts";
import { t } from "./i18n.ts";

// The budgets and what this period has spent of each (PRV-017).
export default function Budgets({ onOpenPrices, profileName }: { onOpenPrices?: () => void; profileName?: (id: string) => string }) {
  const [list, setList] = useState<BudgetState[] | null>(null);
  useEffect(() => {
    const load = () =>
      EngineService.Budgets()
        .then((l) => setList(l ?? []))
        .catch(() => setList([]));
    load();
    const every = setInterval(load, 60_000);
    return () => clearInterval(every);
  }, []);
  if (!list) return null;
  if (list.length === 0) {
    return onOpenPrices ? (
      <p className="muted usage-note">
        {t("budgetsPage.none")}{" "}
        <button className="link" onClick={onOpenPrices}>
          {t("budgetsPage.setOne")}
        </button>
      </p>
    ) : null;
  }
  return (
    <div className="usage-card budgets" aria-label={t("budgetsPage.label")}>
      {list.map((b, i) => (
        <div key={i} className={`budget budget-${b.level || "ok"}`}>
          <div className="budget-head">
            <span>{budgetName(b, profileName?.(b.profileId))}</span>
            <span className="muted">{budgetLine(b)}</span>
          </div>
          <meter min={0} max={b.amount} low={b.amount * 0.8} high={b.amount * 0.8} optimum={0} value={Math.min(b.spent, b.amount)} aria-label={budgetName(b)} />
        </div>
      ))}
    </div>
  );
}
