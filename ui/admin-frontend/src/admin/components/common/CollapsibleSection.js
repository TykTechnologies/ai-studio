import React, { useState } from "react";
import { Typography } from "@mui/material";
import KeyboardArrowDownIcon from "@mui/icons-material/KeyboardArrowDown";
import KeyboardArrowUpIcon from "@mui/icons-material/KeyboardArrowUp";
import { SectionContainer, SectionHeader, SectionContent } from "../../styles/sharedStyles";
const CollapsibleSection = ({ title, children, defaultExpanded = true, sx = {} }) => {
  const [isExpanded, setIsExpanded] = useState(defaultExpanded);

  const toggleExpand = () => {
    setIsExpanded(!isExpanded);
  };

  return (
    <SectionContainer sx={sx}>
      <SectionHeader
        onClick={toggleExpand}
        isExpanded={isExpanded}
        isCollapsible={true}
        sx={{ mx: 2, px: 0, my: 1 }}
      >
        <Typography variant="headingMedium" color="text.primary">
          {title}
        </Typography>
        {isExpanded ? <KeyboardArrowUpIcon /> : <KeyboardArrowDownIcon />}
      </SectionHeader>
      {isExpanded && <SectionContent>{children}</SectionContent>}
    </SectionContainer>
  );
};

export default CollapsibleSection;