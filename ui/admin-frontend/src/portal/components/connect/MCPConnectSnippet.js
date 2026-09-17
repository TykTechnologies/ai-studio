import React from "react";
import { Box, Chip, Typography } from "@mui/material";
import { CopyableBlock } from "./CopyableCode";
import { mcpClientConfig } from "./mcpConfig";

/** Says who serves an endpoint, wherever a Tool or an MCP server is shown. */
export const ServedByChip = ({ label }) => (
  <Chip size="small" variant="outlined" label={label} data-testid="served-by" />
);

/**
 * The MCP client configuration for a set of endpoints, as one block to paste
 * into a client such as Claude Desktop. `entries` come from toolEntry and
 * mcpServerEntry in mcpConfig.js.
 */
const MCPConnectSnippet = ({ entries, title = "MCP client configuration", hint, testId = "mcp-connect-snippet" }) => {
  const usable = (entries || []).filter((entry) => entry && entry.url);
  if (usable.length === 0) return null;
  return (
    <Box>
      <Typography variant="subtitle2" gutterBottom>
        {title}
      </Typography>
      {hint && (
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          {hint}
        </Typography>
      )}
      <CopyableBlock value={mcpClientConfig(usable)} label="MCP client configuration" testId={testId} />
    </Box>
  );
};

export default MCPConnectSnippet;
