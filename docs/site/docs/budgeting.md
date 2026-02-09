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

## Enforcement

> **Note:** Budget *enforcement* (blocking requests when limits are exceeded) is an **Enterprise Edition** feature. In Community Edition, budgets are tracked and recorded for reporting purposes, but requests are not blocked when limits are exceeded.

Budget enforcement primarily occurs at the **[Proxy & API Gateway](./proxy.md)**:

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

This provides **eventually-accurate** budget control. There may be a slight overrun window under very high concurrent load across multiple gateways, but the system converges quickly and prevents sustained overspending.

> **Note:** Budget *enforcement* (blocking requests at the limit) is an Enterprise Edition feature. In Community Edition, budgets are tracked and visible in dashboards but requests are not blocked.

## Integration with Other Systems

*   **[Analytics & Monitoring](./analytics.md):** The Analytics system provides the cost data used to track spending against budgets. The current spent amount for a budget period is derived from aggregated analytics data.
*   **[Model Pricing](./llm-management.md#model-pricing-system):** The pricing definitions are essential for the Analytics system to calculate costs accurately, which in turn feeds the Budget Control system.
*   **[Notification System](./notifications.md):** Budgets trigger notifications when spending reaches defined thresholds. The system supports alerts at **50%**, **80%**, **90%**, and **100%** of the budget limit. Administrators receive notifications when these thresholds are crossed.

## Benefits

*   **Financial Control:** Prevents unexpected high bills from LLM usage.
*   **Resource Management:** Ensures fair distribution of AI resources according to allocated budgets.
*   **Accountability:** Tracks spending against specific configurations or organizational units.

Budget Control is a critical feature for organizations looking to adopt AI technologies responsibly and manage their operational costs effectively.
