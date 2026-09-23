# Budget Control

Tyk AI Studio provides a Budget Control system to help organizations manage and limit spending on Large Language Model (LLM) usage.

## Purpose

The primary goals of the Budget Control system are:

*   **Prevent Overspending:** Set hard limits on costs associated with LLM API calls.
*   **Cost Allocation:** Track and enforce spending limits at different granularities (e.g., per organization, per specific LLM configuration).
*   **Predictability:** Provide better predictability for monthly AI operational costs.

## Scope & Configuration

Budgets are typically configured by administrators and applied at specific levels:

*   **Organization Level:** A global budget limit for all LLM usage within the organization.
*   **LLM Configuration Level:** A specific budget limit tied to a particular LLM setup (e.g., a dedicated budget for a high-cost `gpt-4` configuration).
*   **(Potentially) Application/User Level:** Granular budgets might be assignable to specific applications or user groups (depending on implementation specifics).

**Configuration Parameters:**

*   **Limit Amount:** The maximum monetary value allowed (e.g., $500).
*   **Currency:** The currency the budget is defined in (e.g., USD).
*   **Time Period:** The reset interval for the budget, typically monthly (e.g., resets on the 1st of each month).
*   **Scope:** Which entity the budget applies to (Organization, specific LLM Configuration ID, etc.).

Administrators configure these budgets via the Tyk AI Studio UI or API.

## No Limit Versus Zero

A budget can be empty or a number, and the two mean different things:

*   **No limit** (empty; `null` in the API): spending is not capped.
*   **$0**: nothing may be spent. Requests are refused with HTTP 403.
*   **Any other amount**: requests are refused once that much has been spent in the current period.

In the UI the budget field is an explicit choice between "No limit" and "Fixed amount". On a new App the first choice is "Default": the App's team's allocation when the team has a budget pool, otherwise the platform default (`DEFAULT_APP_BUDGET`), otherwise no limit.

> **Upgrading from earlier versions:** a budget of 0 used to mean "no limit". When the new version first starts, stored App and LLM budgets of 0 are changed to "no limit" automatically, so nothing starts blocking. Scripts and integrations that send `monthly_budget: 0` to mean "no limit" must send `null` (or leave the field out) instead.

## Enforcement

> **Note:** Budget *enforcement* (blocking requests when limits are exceeded) is an **Enterprise Edition** feature. In Community Edition, budgets are tracked and recorded for reporting purposes, but requests are not blocked when limits are exceeded.

Budget enforcement primarily occurs at the **[Proxy & API Gateway](./proxy.md)**:

Most deployments serve traffic through [Microgateways](./edge-gateways.md); Studio's embedded gateway applies the same rules.

1.  **Request Received:** The Proxy receives a request destined for an LLM.
2.  **Cost Estimation:** Before forwarding the request, the Proxy might estimate the potential maximum cost (or rely on post-request cost calculation).
3.  **Budget Check:** The Proxy checks the current spending against all applicable budgets (e.g., the specific LLM config budget AND the overall organization budget) for the current time period.
4.  **Allow or Deny (Enterprise Edition):**
    *   If the current spending plus the estimated/actual cost of the request does *not* exceed the limit(s), the request is allowed to proceed.
    *   If the request *would* cause a budget limit to be exceeded, the request is blocked with HTTP 403, and an error is returned to the caller.

## Distributed Budget Control (Multi-Gateway)

When running multiple Microgateways in a hub-and-spoke architecture, budget tracking faces a split-brain challenge — each gateway only has local visibility into its own spend. Tyk AI Studio solves this with a **budget pulse** mechanism:

1. **Analytics batching:** All Microgateways send analytics records (including cost data) back to AI Studio in regular batches. This gives AI Studio a **complete view** of token spend across the entire estate.

2. **Budget pulse:** AI Studio periodically sends a budget pulse to each gateway containing the **total spend** for each access token across all gateways.

3. **Local update:** Each gateway updates its local spend counter if Studio's reported number is higher than what it has locally.

4. **Blocks:** the same pulse lists the Apps every gateway must refuse regardless of its local numbers: Apps with a budget of $0, and Apps whose [team](#team-budgets-enterprise) has spent a blocking team budget. Gateways keep this list across restarts.

5. **Alerts:** each pulse also has AI Studio check the Apps whose spend moved at the gateways against their App, LLM and team budgets, so the 80% and 100% alerts fire for gateway traffic too.

This provides **eventually-accurate** budget control. There may be a slight overrun window under very high concurrent load across multiple gateways, but the system converges quickly and prevents sustained overspending.

> **Note:** Budget *enforcement* (blocking requests at the limit) is an Enterprise Edition feature. In Community Edition, budgets are tracked and visible in dashboards but requests are not blocked.

## Team Budgets (Enterprise)

Budgets can also be set per **team**. A team budget is both:

*   **A ceiling:** the most all of the team's Apps may spend together in a month. It can alert only, or also block the team's Apps once it is reached.
*   **An allocation pool:** when an App is created for a member of the team, it gets a default allocation from the pool, capped at what is left. When the pool is empty, the App gets nothing and its requests are refused until an administrator allocates to it. Deleting an App returns its allocation to the pool; the money it already spent this month still counts towards the team.

**Which team an App belongs to.** Users can belong to several teams, so each App is attributed to one team when it is created:

1.  the owner's **budget team**, if one is set on the user and they are still a member;
2.  otherwise their first team other than Default;
3.  otherwise the **Default** team.

Administrators can choose another team on the App form.

**Switching it on.** Team budgets are off until an administrator turns on the **Team budgets** switch on the Teams page. When it is on:

*   the Default team starts with an empty pool (a budget of 0), so Apps that fall through to it get nothing until it is given a budget;
*   Apps created before the switch keep their own budgets;
*   a team without a budget is not affected.

**Seeing what a team costs.** Every team's spend is reported whether or not it has a budget:

*   the **Team Costs** table on the dashboard covers the selected date range;
*   each team's page shows its budget, spend, allocations and the per-App breakdown for the current period.

**Overshoot.** Administrators are notified at 80% and 100% of a team's budget. The same thresholds publish `budget.team.threshold` events, which can be sent to [webhooks](./webhooks.md), and are recorded in the audit trail. The team page also warns when the App allocations add up to more than the team budget.

Edge gateways receive the list of Apps their team blocks with each budget pulse, so a team block reaches the edges within one sync interval (30 seconds by default).

## Integration with Other Systems

*   **[Analytics & Monitoring](./analytics.md):** The Analytics system provides the cost data used to track spending against budgets. The current spent amount for a budget period is derived from aggregated analytics data.
*   **[Model Pricing](./llm-management.md#model-pricing-system):** The pricing definitions are essential for the Analytics system to calculate costs accurately, which in turn feeds the Budget Control system.
*   **[Notification System](./notifications.md):** Budgets trigger notifications when spending reaches defined thresholds. Alerts are sent at **80%** (warning) and **100%** (limit reached) of the budget, once per threshold per budget period, to the App owner and administrators.

## Benefits

*   **Financial Control:** Prevents unexpected high bills from LLM usage.
*   **Resource Management:** Ensures fair distribution of AI resources according to allocated budgets.
*   **Accountability:** Tracks spending against specific configurations or organizational units.

Budget Control is a critical feature for organizations looking to adopt AI technologies responsibly and manage their operational costs effectively.
