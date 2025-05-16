import React from "react";
import { Typography } from "@mui/material";
import { SectionContainer, SectionHeader, SectionContent } from "../../styles/sharedStyles";

const Section = ({ title, children, sx = {} }) => {
  return (
    <SectionContainer sx={sx}>
        { title && (
        <SectionHeader isCollapsible={false} sx={{ mx: 2, px: 0 }}>
            <Typography variant="headingMedium" color="text.primary">
            {title}
            </Typography>
        </SectionHeader>
        )}
      <SectionContent>{children}</SectionContent>
    </SectionContainer>
  );
};

export default Section;