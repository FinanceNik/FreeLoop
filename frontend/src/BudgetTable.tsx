import type { Budget, Profile } from "./api.ts";
import { t } from "./i18n.ts";

// The budgets to edit beside the prices (PRV-017): an amount a day or a month, for
// every session or one profile's.
export default function BudgetTable({ budgets, profiles, onChange }: { budgets: Budget[]; profiles: Profile[]; onChange: (b: Budget[]) => void }) {
  const set = (i: number, patch: Partial<Budget>) => onChange(budgets.map((b, j) => (j === i ? { ...b, ...patch } : b)));
  return (
    <>
      <p className="muted">{t("budgets.intro")}</p>
      {budgets.map((b, i) => (
        <div key={i} className="row budget-row">
          <select aria-label={t("budgets.period")} value={b.period} onChange={(e) => set(i, { period: e.target.value })}>
            <option value="day">{t("budgets.perDay")}</option>
            <option value="month">{t("budgets.perMonth")}</option>
          </select>
          <input
            aria-label={t("budgets.amount")}
            type="number"
            min="0.01"
            step="0.01"
            value={b.amount || ""}
            placeholder="20.00"
            onChange={(e) => set(i, { amount: Number(e.target.value) })}
          />
          <select aria-label={t("budgets.for")} value={b.profileId} onChange={(e) => set(i, { profileId: e.target.value })}>
            <option value="">{t("budgets.everySession")}</option>
            {profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <button type="button" className="danger" onClick={() => onChange(budgets.filter((_, j) => j !== i))}>
            {t("common.remove")}
          </button>
        </div>
      ))}
      <button type="button" onClick={() => onChange([...budgets, { period: "day", amount: 0, profileId: "" }])}>
        {t("budgets.add")}
      </button>
    </>
  );
}
