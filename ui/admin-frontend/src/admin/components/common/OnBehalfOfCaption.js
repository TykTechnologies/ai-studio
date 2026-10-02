import React from "react";
import { Box, Typography } from "@mui/material";

/**
 * Who a proxied call was for and the agent that made it, when an edge auth
 * plugin said (a delegated token's subject and its act/azp). Rendered under a
 * proxy log row's timestamp; nothing when neither is set. Audit information:
 * the App and its owner are still who the call is billed and authorised as.
 */
const OnBehalfOfCaption = ({ attributes }) => {
  const onBehalfOf = attributes?.on_behalf_of;
  const actingAgent = attributes?.acting_agent;
  if (!onBehalfOf && !actingAgent) return null;
  return (
    <Box sx={{ mt: 0.5 }} data-testid="on-behalf-of">
      {onBehalfOf && (
        <Typography variant="caption" color="text.secondary" component="div" sx={{ wordBreak: "break-all" }}>
          For: {onBehalfOf}
        </Typography>
      )}
      {actingAgent && (
        <Typography variant="caption" color="text.secondary" component="div" sx={{ wordBreak: "break-all" }}>
          Agent: {actingAgent}
        </Typography>
      )}
    </Box>
  );
};

export default OnBehalfOfCaption;
