import React from "react";
import { Box, Typography } from "@mui/material";

/**
 * A proxy log row's request ids, under its timestamp: the gateway's own (the
 * X-Tyk-AI-Request-ID it answered with, also on the request's compliance
 * events) and the caller's X-Request-ID when it sent one. Nothing for rows
 * from before request ids were recorded.
 */
const RequestIdCaption = ({ attributes }) => {
  const requestId = attributes?.request_id;
  const clientId = attributes?.client_request_id;
  if (!requestId && !clientId) return null;
  const mono = { fontFamily: "monospace", wordBreak: "break-all" };
  return (
    <Box sx={{ mt: 0.5 }} data-testid="request-id">
      {requestId && (
        <Typography variant="caption" color="text.secondary" component="div" sx={mono}>
          {requestId}
        </Typography>
      )}
      {clientId && (
        <Typography variant="caption" color="text.secondary" component="div" sx={mono}>
          Client: {clientId}
        </Typography>
      )}
    </Box>
  );
};

export default RequestIdCaption;
