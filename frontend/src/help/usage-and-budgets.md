# Usage and budgets

See what your sessions use and cost, and set budgets that warn you.

## Open the usage views

The **Usage** button in the sidebar opens the usage page. Drag it onto a pane to watch usage beside your sessions. In a pane, arrows step through Recent hours, Last 12 months, Tokens and By model.

Once anything used today has a price, or a budget is near, the sidebar also shows today's estimated cost.

## Read the charts

| Tab | Shows |
| --- | --- |
| Recent hours | The last 12 or 24 hours, following model calls as they finish. |
| Tokens and cost | 7 days to 12 months, compared with the period before. |
| Activity, Approvals, Efficiency | What agents did, who decided each approval, what a turn takes. |

On **Tokens and cost**, **Group by** splits the chart by token type, model, provider or agent, or project. Filters apply to every tab, and **Clear filters** removes them. **Export CSV…** saves the data.

## What estimated means

Freeloop counts the tokens of every model call, including those of external agents that report them. A cost is an estimate: tokens multiplied by a price in US dollars per million tokens.

Prices come from **Settings → Costs**. Anthropic, OpenAI and Google models use their official API list prices. A row you add under Prices comes first, so you can enter your own price or one for a model that isn't listed. A model can be a pattern such as `claude-*`, and the first matching row applies.

- A model with no price counts as no cost. The page says how many tokens that is and offers **Add prices**.
- Some agents report no tokens for a turn, or only a total. A note says which, and the cost leaves those turns out.
- An agent that reports tokens but not its model, such as Gemini CLI, can be priced anyway. Set **Model it runs on** in its profile on the Profiles page.
- A figure that leaves something out ends in **+**.

> If nothing in the range is priced, the Cost choice is disabled until you add a price.

## Agent limits

The Limits section shows how much of an account limit an agent has reported using, when it resets and how old the report is. A limit can read **Close to the limit** or **Limit reached**.

Freeloop asks no vendor for these numbers. They come with an agent's own model calls, so a limit appears only after the agent's next call. API keys and local models report none.

## Set budgets

A budget is an amount of estimated spending. Under **Settings → Costs → Budgets**, click **Add a budget** and set:

| Field | Meaning |
| --- | --- |
| Period | Per day or Per month, in your local time. |
| Amount in US dollars | The limit. |
| For | Every session, or one profile. |

A budget counts only priced usage, and its line says what it leaves out. Each budget shows a meter on the Tokens and cost tab.

- From 80%, the usage page and the sidebar mark the budget as near, and you get a notification once per period if notifications are on.
- Once the amount is reached, the same happens again and the queue starts no new task under that budget.
- The queue also waits when an agent's limit is close or reached.

> Running sessions are never stopped. A budget warns and holds the queue, nothing more.
