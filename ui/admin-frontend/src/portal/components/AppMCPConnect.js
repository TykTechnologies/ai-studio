import React, { useState } from "react";
import { Box, Card, CardContent, Typography } from "@mui/material";
import AppMCPAccess from "./AppMCPAccess";
import CopyableCode from "./connect/CopyableCode";
import MCPConnectSnippet, { ServedByChip } from "./connect/MCPConnectSnippet";
import { SERVED_BY, mcpServerEntry, toolEntry } from "./connect/mcpConfig";
import { toolMcpEnabled, toolMcpEndpoint } from "../utils/toolEndpoints";

/**
 * AppMCPConnect is the "Connect via MCP" section of the portal App page.
 *
 * An App can reach two kinds of thing over MCP, and the page used to describe
 * them in two unrelated places with two different config snippets:
 *   - its Tools with MCP access on: served by AI Studio, opened with the
 *     App's own credential;
 *   - its MCP servers: served by a Tyk Gateway, opened with a Tyk access key
 *     the owner requests here.
 * A developer configures both in the same MCP client, so they are shown
 * together, each labelled with who serves it and which credential it takes,
 * followed by one configuration block covering all of them.
 */
const AppMCPConnect = ({ appId, credential, showSecret, tools = [], mcpServers = [], toolBaseUrl = "" }) => {
  // AppMCPAccess loads the servers with the header each proxy expects; the
  // App payload does not carry it.
  const [summary, setSummary] = useState(null);

  const mcpTools = tools
    .filter((tool) => toolMcpEnabled(tool.attributes))
    .map((tool) => ({ ...tool.attributes, id: tool.id, mcp_endpoint_url: toolMcpEndpoint(tool.attributes, toolBaseUrl) }));
  if (mcpTools.length === 0 && mcpServers.length === 0) return null;

  const keyedServers = (summary?.servers || mcpServers).filter((server) => server.brokerable !== false && server.endpoint_url);
  const secret = showSecret ? credential?.secret : "";
  const entries = [...mcpTools.map((tool) => toolEntry(tool, secret)), ...keyedServers.map((server) => mcpServerEntry(server))];

  return (
    <Box data-testid="app-mcp-connect">
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Everything in this app an MCP client can connect to. Tools are served by AI Studio and use this app's credential. MCP servers are
        served by a Tyk Gateway and use a Tyk access key.
      </Typography>

      {mcpTools.length > 0 && (
        <Card sx={{ mb: 2 }} data-testid="mcp-connect-tools">
          <CardContent>
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1, flexWrap: "wrap" }}>
              <Typography variant="h6">Tools</Typography>
              <ServedByChip label={SERVED_BY.studio} />
            </Box>
            {mcpTools.map((tool) => (
              <Box key={tool.id} sx={{ mb: 1.5 }} data-testid={`mcp-connect-tool-${tool.id}`}>
                <Typography variant="subtitle2">{tool.name}</Typography>
                <CopyableCode value={tool.mcp_endpoint_url} label={`MCP endpoint of ${tool.name}`} />
              </Box>
            ))}
            <Typography variant="body2" color="text.secondary">
              Authenticate with this app's credential in the <code>Authorization</code> header as a bearer token. MCP clients that support
              OAuth can sign in instead and choose this app.
              {credential?.active === false && " The credential works once the app is approved."}
            </Typography>
          </CardContent>
        </Card>
      )}

      {mcpServers.length > 0 && (
        <Box data-testid="mcp-connect-servers">
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1, flexWrap: "wrap" }}>
            <Typography variant="h6">MCP servers</Typography>
            <ServedByChip label={SERVED_BY.tyk} />
          </Box>
          <AppMCPAccess appId={appId} credentialActive={!!credential?.active} onSummary={setSummary} />
        </Box>
      )}

      <MCPConnectSnippet
        entries={entries}
        hint={
          keyedServers.length > 0
            ? "One file for every endpoint above. A Tyk access key is shown only when it is issued, so paste yours in place of the placeholder."
            : showSecret
              ? "Paste this into your MCP client, for example Claude Desktop."
              : "Paste this into your MCP client, for example Claude Desktop. Reveal the app secret above to fill it in."
        }
      />
    </Box>
  );
};

export default AppMCPConnect;
