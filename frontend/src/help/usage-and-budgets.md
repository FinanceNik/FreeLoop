# Usage and budgets

This page explains how to read the usage views, what an estimated cost means, and how to set budgets.

## Where to look

The **Usage** button in the sidebar opens the usage page. Drag the same button onto a pane to watch usage beside your sessions, or click **Show in a pane** on the page. In a pane, the arrows beside its name step through four views: **Recent hours**, **Last 12 months**, **Tokens** and **By model**.

Once anything used today has a price, or a budget is near, the sidebar also shows today's estimated cost. Click it to open the usage page.

## Recent hours

**Recent hours** charts the last 12 or 24 hours in steps of a few minutes. It follows model calls as they finish. Choose **Tokens** or **Cost**, and tick **Cache reads** to include them.

## Tokens and cost

The **Tokens and cost** tab shows a range of **7 days**, **30 days**, **90 days** or **12 months**, compared with the period before. Use **Group by** to split the chart by **Token type**, **Model**, **Provider or agent** or **Project**. The five largest groups get a colour each, and the rest fold into **Other**.

Three more tabs, **Activity**, **Approvals** and **Efficiency**, show what the agents did, who decided each approval, and what a turn takes. They use the same range and filters.

The filters apply to every tab. Choose a project, provider or agent, model or tag, and use **Clear filters** to remove them. Clicking a name in a table filters by it.

Below the chart you find tables by token type, by model (or the group you chose) and the sessions that used the most. **Share summary** prepares a text summary, and **Export CSV…** saves the data. **Show as a table** lists the chart's numbers.

## What estimated means

Freeloop counts the tokens of every model call, including those of external agents that report them. A cost is an estimate: tokens multiplied by a price.

Prices come from **Settings → Costs**. Models from Anthropic, OpenAI and Google use their official API list prices, shown with the date they were last updated. A row you add under **Prices** comes first, so you can enter your own price or a price for a model that is not on the list. A model can be a pattern such as `claude-*`, and the first matching row applies. Prices are US dollars per million tokens.

Some usage cannot be priced or counted:

* Calls to a model with no price count as no cost. The page says how many tokens that is and offers **Add prices**.
* Some agents report no tokens for a turn, or only a total for the turn. A note on the page says which, and the cost leaves those turns out.
* An agent that reports tokens but not its model, such as Gemini CLI, can be priced anyway: set **Model it runs on** in its profile on the Profiles page. The agent is not told; a model it reports wins.
* A figure that leaves something out ends in **+**. The sidebar's cost and the **Recent hours** cost both do this.

If nothing in the range is priced, the **Cost** choice on the **Tokens and cost** tab is disabled until you add a price.

## Limits

The **Limits** section shows how much of an account limit an agent has reported using, with a bar, when it resets and how old the report is. A limit can read **Close to the limit** or **Limit reached**. A limit that has reset shows no figure until the agent reports again.

Freeloop asks no vendor for these numbers. They come with an agent's own model calls, so a limit appears only after the agent's next call. API keys and local models report none. The page also summarizes finished runs of each limit over the last 90 days.

## Budgets

A budget is an amount of estimated spending. Add one under **Settings → Costs → Budgets** with **Add a budget**:

* **Period** is **Per day** or **Per month**, in your local time.
* **Amount in US dollars** is the limit.
* **For** is **Every session** or one profile.

Budgets use the same prices as the cost estimate, so a budget only counts priced usage. A budget line says what it does not count: models without a price, or turns an agent did not report.

Each budget shows a meter on the **Tokens and cost** tab with the spending so far.

* From 80%, the usage page and the sidebar's cost mark the budget as near, and you get a notification once per period if notifications are on.
* Once the amount is reached, the same happens again and the queue starts no new task under that budget.
* Running sessions are never stopped. A budget warns and holds the queue, nothing more.

Agent limits work alongside budgets. The queue also waits when an agent's limit is close or reached.
