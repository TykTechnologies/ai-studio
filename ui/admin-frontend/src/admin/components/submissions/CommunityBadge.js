import React from "react";
import { Chip } from "@mui/material";
import PeopleIcon from "@mui/icons-material/People";

const CommunityBadge = ({ show = true, size = "small" }) => {
  if (!show) return null;

  return (
    <Chip
      label="Community"
      size={size}
      icon={<PeopleIcon sx={{ fontSize: "0.9rem" }} />}
      sx={{
        backgroundColor: "#e8f5e9",
        color: "#2e7d32",
        fontWeight: "bold",
        fontSize: "0.7rem",
        "& .MuiChip-icon": {
          color: "#2e7d32",
        },
      }}
    />
  );
};

export default CommunityBadge;
