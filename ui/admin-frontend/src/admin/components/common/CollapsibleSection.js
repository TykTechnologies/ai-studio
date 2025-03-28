import React, { useState } from "react";
import { Box, Typography, Paper } from "@mui/material";
import { styled } from "@mui/material/styles";
import KeyboardArrowDownIcon from "@mui/icons-material/KeyboardArrowDown";
import KeyboardArrowUpIcon from "@mui/icons-material/KeyboardArrowUp";

const SectionContainer = styled(Paper)(({ theme }) => ({
  border: `1px solid ${theme.palette.border.neutralDefault}`,
  borderRadius: "8px",
  overflow: "hidden",
  marginBottom: theme.spacing(2),
  boxShadow: "none",
}));

const SectionHeader = styled(Box)(({ theme, isExpanded }) => ({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  padding: theme.spacing(2),
  cursor: "pointer",
  borderBottom: isExpanded ? `1px solid ${theme.palette.border.neutralHovered}` : "none",
}));

const SectionContent = styled(Box)(({ theme }) => ({
  padding: theme.spacing(3),
}));

/**
 * A reusable collapsible section component
 * 
 * @param {Object} props - Component props
 * @param {string} props.title - Section title
 * @param {React.ReactNode} props.children - Section content
 * @param {boolean} [props.defaultExpanded=true] - Whether the section is expanded by default
 * @param {Object} [props.sx] - Additional styles for the container
 * @returns {React.ReactElement}
 */
const CollapsibleSection = ({ title, children, defaultExpanded = true, sx = {} }) => {
  const [isExpanded, setIsExpanded] = useState(defaultExpanded);

  const toggleExpand = () => {
    setIsExpanded(!isExpanded);
  };

  return (
    <SectionContainer sx={sx}>
      <SectionHeader onClick={toggleExpand} isExpanded={isExpanded} sx={{ mx: 2, px: 0 }}>
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