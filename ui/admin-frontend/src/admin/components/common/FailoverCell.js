import React from "react";

// formatFailover describes a proxy-log row's place in a failover waterfall.
// The proxy-log endpoints carry failover_attempt (1-based rung index, 0 for
// the primary attempt) and failover_from_llm_id (the primary the request
// failed over from). A primary attempt is shown as a dash so the column
// reads as "which rows are fallbacks" at a glance.
export function formatFailover(attributes) {
  const attempt = Number(attributes?.failover_attempt) || 0;
  if (attempt <= 0) {
    return "-";
  }
  const from = attributes?.failover_from_llm_id;
  if (from === undefined || from === null) {
    return `attempt ${attempt}`;
  }
  return `attempt ${attempt} (from LLM #${from})`;
}

// FailoverCell renders formatFailover for a proxy-log row's attributes.
const FailoverCell = ({ attributes }) => (
  <span data-testid="proxy-log-failover">{formatFailover(attributes)}</span>
);

export default FailoverCell;
