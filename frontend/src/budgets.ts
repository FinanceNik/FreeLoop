// Budgets in words (PRV-017). Pure, so budgets.test.ts covers it.
import { formatCost } from "./cost.ts";
import { t } from "./i18n.ts";

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
  const amount = formatCost(b.amount);
  if (b.period === "month") return b.profileId ? t("app.budget.monthlyFor", { amount, profile: profileName }) : t("app.budget.monthly", { amount });
  return b.profileId ? t("app.budget.dailyFor", { amount, profile: profileName }) : t("app.budget.daily", { amount });
}

// budgetLine says what was spent and what the figure leaves out.
export function budgetLine(b: BudgetLike): string {
  const share = b.amount > 0 ? Math.round((100 * b.spent) / b.amount) : 0;
  const cost = formatCost(b.spent);
  const count = b.unreported;
  const line =
    b.unpriced && count > 0
      ? t("app.budget.spentBoth", { cost, share, count })
      : b.unpriced
        ? t("app.budget.spentUnpriced", { cost, share })
        : count > 0
          ? t("app.budget.spentUnreported", { cost, share, count })
          : t("app.budget.spent", { cost, share });
  return b.level === "reached" ? t("app.budget.reached", { line }) : line;
}

// worst is the level the sidebar's cost shows: reached over warning over none.
export function worst(budgets: readonly Pick<BudgetLike, "level">[]): string {
  if (budgets.some((b) => b.level === "reached")) return "reached";
  return budgets.some((b) => b.level === "warning") ? "warning" : "";
}
