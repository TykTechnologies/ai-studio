import React, { useState } from "react";
import { Box, Typography, Stack } from "@mui/material";
import KeyboardArrowDownIcon from "@mui/icons-material/KeyboardArrowDown";
import KeyboardArrowUpIcon from "@mui/icons-material/KeyboardArrowUp";

const AdvancedSettingsSection = ({ children }) => {
  const [isExpanded, setIsExpanded] = useState(false);

  const toggleExpand = () => {
    setIsExpanded(!isExpanded);
  };

  return (
    <Box sx={{ mt: 2 }}>
      <Box 
        onClick={toggleExpand} 
        sx={{ 
          display: "flex", 
          alignItems: "center", 
          cursor: "pointer",
          mb: isExpanded ? 2 : 0
        }}
      >
        {isExpanded ? 
          <KeyboardArrowUpIcon sx={{ color: "text.defaultSubdued" }} /> : 
          <KeyboardArrowDownIcon sx={{ color: "text.defaultSubdued" }} />
        }
        <Typography 
          variant="bodyLargeMedium" 
          color="text.defaultSubdued" 
          sx={{ ml: 1 }}
        >
          Advance settings
        </Typography>
      </Box>
      {isExpanded && (
        <Stack spacing={2}>
          {children}
        </Stack>
      )}
    </Box>
  );
};

export default AdvancedSettingsSection;