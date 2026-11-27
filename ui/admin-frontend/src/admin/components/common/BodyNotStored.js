import React from "react";
import { Typography } from "@mui/material";

// BODY_NOT_STORED_HELP explains why a proxy log row has no body. It is not
// an error: AI Studio stores bodies unless the LLM's body logging is
// disabled, and an edge gateway stores (and forwards to AI Studio) only what
// its own settings allow, off by default.
export const BODY_NOT_STORED_HELP =
  "No body was stored for this request. AI Studio keeps bodies unless the LLM's Disable Request/Response Body Logging option is on. " +
  "Requests served by an edge gateway keep bodies only when that edge sets ANALYTICS_STORE_REQUESTS / ANALYTICS_STORE_RESPONSES " +
  "and its analytics pulse sets include_request_response_data; both are off by default.";

/**
 * BodyNotStored stands in for an empty request or response body in a proxy
 * log table, so an empty cell is not read as an empty request.
 */
const BodyNotStored = () => (
  <Typography variant="caption" color="text.secondary" title={BODY_NOT_STORED_HELP} data-testid="body-not-stored">
    Not stored
  </Typography>
);

export default BodyNotStored;
