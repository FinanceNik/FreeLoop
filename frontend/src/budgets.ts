// Budgets in words (PRV-017). Pure, so budgets.test.ts covers it.
import { formatCost } from "./cost.ts";

export interface BudgetLike {
  period: string;
  amount: number;
  profileId: string;
  spent: number;
  unpriced: boolean;
  unreported: number;
  level: string;
}

export function budgetName(b: Pick<BudgetLike, "period" | "amount" | "profileId">, profileName = b.profileId): string {
  const period = b.period === "month" ? "Monthly" : "Daily";
  return `${period} budget, ${formatCost(b.amount)}${b.profileId ? ` for ${profileName}` : ""}`;
}

// budgetLine says what was spent and what the figure leaves out.
export function budgetLine(b: BudgetLike): string {
  const share = b.amount > 0 ? Math.round((100 * b.spent) / b.amount) : 0;
  let text = `≈ ${formatCost(b.spent)} spent, ${share}%`;
  const missing: string[] = [];
  if (b.unpriced) missing.push("models without a price");
  if (b.unreported > 0) missing.push(`${b.unreported === 1 ? "1 turn" : `${b.unreported} turns`} an agent did not report`);
  if (missing.length) text += `, not counting ${missing.join(" or ")}`;
  if (b.level === "reached") text += ". Reached: the queue waits, running sessions go on.";
  return text;
}

// worst is the level the sidebar's cost shows: reached over warning over none.
export function worst(budgets: readonly Pick<BudgetLike, "level">[]): string {
  if (budgets.some((b) => b.level === "reached")) return "reached";
  return budgets.some((b) => b.level === "warning") ? "warning" : "";
}
