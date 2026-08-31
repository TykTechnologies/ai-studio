import React from "react";
import { Chip } from "@mui/material";

// Exported so anything else naming a submission status -- the contributions
// tabs, for one -- reads from the same place. The tabs used to say "Published"
// where this chip said "Approved", and "published" was wrong either way:
// approval creates the resource but puts it in no catalogue, so nothing is
// published until an administrator assigns one.
export const statusConfig = {
  draft: { color: "#757575", bg: "#f5f5f5", label: "Draft" },
  submitted: { color: "#1565c0", bg: "#e3f2fd", label: "Pending Review" },
  in_review: { color: "#e65100", bg: "#fff3e0", label: "In Review" },
  approved: { color: "#2e7d32", bg: "#e8f5e9", label: "Approved" },
  rejected: { color: "#c62828", bg: "#ffebee", label: "Rejected" },
  changes_requested: {
    color: "#f57f17",
    bg: "#fffde7",
    label: "Changes Requested",
  },
};

export const statusLabel = (status) =>
  statusConfig[status]?.label || status || "Unknown";

const StatusChip = ({ status, size = "small" }) => {
  const config = statusConfig[status] || {
    color: "#757575",
    bg: "#f5f5f5",
    label: status || "Unknown",
  };

  return (
    <Chip
      label={config.label}
      size={size}
      sx={{
        backgroundColor: config.bg,
        color: config.color,
        fontWeight: "bold",
        fontSize: "0.75rem",
      }}
    />
  );
};

export default StatusChip;
