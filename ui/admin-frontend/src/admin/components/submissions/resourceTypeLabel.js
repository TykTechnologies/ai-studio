import React from "react";
import { Chip } from "@mui/material";
import StorageIcon from "@mui/icons-material/Storage";
import BuildIcon from "@mui/icons-material/Build";
import ExtensionIcon from "@mui/icons-material/Extension";
import HubIcon from "@mui/icons-material/Hub";

// A submission's resource type used to be a two-way ternary repeated in every
// list, card and header. Plugin-provided types (ResourceProvider plugins) made
// that a three-way branch with a label that lives on the submission itself, so
// the mapping now has one home.

export const getResourceTypeLabel = (submission) => {
  switch (submission?.resource_type) {
    case "datasource":
      return "Data source";
    case "tool":
      return "Tool";
    case "plugin":
      return submission.plugin_resource_type?.name || "Plugin Resource";
    case "mcp_server":
      return "MCP server";
    default:
      return "Resource";
  }
};

export const getResourceTypeIcon = (submission) => {
  switch (submission?.resource_type) {
    case "datasource":
      return <StorageIcon />;
    case "tool":
      return <BuildIcon />;
    case "mcp_server":
      return <HubIcon />;
    default:
      return <ExtensionIcon />;
  }
};

export const ResourceTypeChip = ({ submission, withIcon = true, ...chipProps }) => (
  <Chip
    icon={withIcon ? getResourceTypeIcon(submission) : undefined}
    label={getResourceTypeLabel(submission)}
    variant="outlined"
    {...chipProps}
  />
);

export default ResourceTypeChip;
