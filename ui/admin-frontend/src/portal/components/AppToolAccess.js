import React from "react";
import { Link } from "react-router-dom";
import { Alert, Box, Button, Card, CardContent, Chip, Typography } from "@mui/material";
import DescriptionIcon from "@mui/icons-material/Description";
import CopyableCode from "./connect/CopyableCode";
import { ServedByChip } from "./connect/MCPConnectSnippet";
import { SERVED_BY } from "./connect/mcpConfig";
import { toolMcpEnabled, toolRestEnabled, toolRestEndpoint } from "../utils/toolEndpoints";

/**
 * AppToolAccess is the "Tool access details" section of the portal App page:
 * one card per tool with the access methods an administrator has switched on.
 * The REST endpoint is shown here; MCP endpoints are collected in the
 * "Connect via MCP" section above, next to the App's MCP servers.
 *
 * A tool can lose an access method after it was added to the App. It stays
 * listed, with a note, so the owner can see why calls are now refused.
 */
const AppToolAccess = ({ tools = [], toolBaseUrl = "" }) => {
  if (tools.length === 0) {
    return <Typography variant="body1">No tools associated with this app.</Typography>;
  }
  return tools.map((tool) => {
    const attrs = tool.attributes || {};
    const rest = toolRestEnabled(attrs);
    const mcp = toolMcpEnabled(attrs);
    return (
      <Card key={tool.id} sx={{ mb: 3 }} data-testid={`app-tool-${tool.id}`}>
        <CardContent>
          <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
            <Typography variant="h6">{attrs.name}</Typography>
            <ServedByChip label={SERVED_BY.studio} />
            {rest && <Chip size="small" label="REST API" color="primary" />}
            {mcp && <Chip size="small" label="MCP" color="primary" />}
          </Box>
          <Typography variant="body2" color="text.secondary" mb={2}>
            {/* A Tool is serialised with `description`; only datasources carry
                `short_description`. */}
            {attrs.short_description || attrs.description || "No description available"}
          </Typography>

          {!rest && !mcp && (
            <Alert severity="warning" data-testid={`app-tool-unreachable-${tool.id}`}>
              An administrator has switched off REST API and MCP access for this tool. It is available in chat only, and calls from this
              app are refused.
            </Alert>
          )}

          {rest && (
            <>
              <Typography variant="subtitle1" sx={{ fontWeight: "bold", mt: 2, mb: 1 }}>
                REST API
              </Typography>
              <Typography variant="body2" sx={{ mb: 1 }}>
                Call the tool's operations with a POST to this URL, authenticated with this app's credential.
              </Typography>
              <CopyableCode value={toolRestEndpoint(attrs, toolBaseUrl)} label={`REST endpoint of ${attrs.name}`} testId={`app-tool-rest-${tool.id}`} />
            </>
          )}
          {!rest && mcp && (
            <Typography variant="body2" color="text.secondary">
              This tool is reached over MCP only. Its endpoint is under Connect via MCP above.
            </Typography>
          )}

          {(rest || mcp) && (
            <Box sx={{ mt: 2, display: "flex", justifyContent: "flex-end" }}>
              <Button
                component={Link}
                to={`/portal/tools/${tool.id}/docs`}
                variant="outlined"
                color="primary"
                size="small"
                startIcon={<DescriptionIcon />}
              >
                View Documentation
              </Button>
            </Box>
          )}
        </CardContent>
      </Card>
    );
  });
};

export default AppToolAccess;
