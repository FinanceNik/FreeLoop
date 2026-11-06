import type { Budget, Profile } from "./api.ts";

// The budgets to edit beside the prices (PRV-017): an amount a day or a month, for
// every session or one profile's.
export default function BudgetTable({ budgets, profiles, onChange }: { budgets: Budget[]; profiles: Profile[]; onChange: (b: Budget[]) => void }) {
  const set = (i: number, patch: Partial<Budget>) => onChange(budgets.map((b, j) => (j === i ? { ...b, ...patch } : b)));
  return (
    <>
      <p className="muted">
        Estimated spending at the prices below. From 80% the usage page and the sidebar's cost say so and a notification comes; once reached, the queue starts no
        new task. Running sessions are never stopped.
      </p>
      {budgets.map((b, i) => (
        <div key={i} className="row budget-row">
          <select aria-label="Period" value={b.period} onChange={(e) => set(i, { period: e.target.value })}>
            <option value="day">Per day</option>
            <option value="month">Per month</option>
          </select>
          <input
            aria-label="Amount in US dollars"
            type="number"
            min="0.01"
            step="0.01"
            value={b.amount || ""}
            placeholder="20.00"
            onChange={(e) => set(i, { amount: Number(e.target.value) })}
          />
          <select aria-label="For" value={b.profileId} onChange={(e) => set(i, { profileId: e.target.value })}>
            <option value="">Every session</option>
            {profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <button type="button" className="danger" onClick={() => onChange(budgets.filter((_, j) => j !== i))}>
            Remove
          </button>
        </div>
      ))}
      <button type="button" onClick={() => onChange([...budgets, { period: "day", amount: 0, profileId: "" }])}>
        Add a budget
      </button>
    </>
  );
}
