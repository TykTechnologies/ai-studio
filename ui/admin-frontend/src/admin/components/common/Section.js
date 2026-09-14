import React from "react";
import { Box, Typography } from "@mui/material";
import { SectionContainer, SectionHeader, SectionContent } from "../../styles/sharedStyles";

/**
 * One bordered section for detail pages: a titled header, optional one-line
 * description under the title, optional actions on the right of the header
 * (an Export button, a link), and the content below. Pages that used to
 * hand-roll an `h6` per block use this instead so every detail page reads
 * the same way (UX review M5). `title` is optional so the plain, header-less
 * usage on the team page keeps working.
 */
const Section = ({ title, description, actions, children, sx = {}, contentSx = {}, ...rest }) => {
  const hasHeader = Boolean(title || actions);
  return (
    <SectionContainer sx={sx} {...rest}>
      {hasHeader && (
        <SectionHeader isCollapsible={false} sx={{ mx: 2, px: 0 }}>
          <Box sx={{ minWidth: 0 }}>
            {title && (
              <Typography variant="headingMedium" color="text.primary" component="h2">
                {title}
              </Typography>
            )}
            {description && (
              <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                {description}
              </Typography>
            )}
          </Box>
          {actions && (
            <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexShrink: 0 }}>{actions}</Box>
          )}
        </SectionHeader>
      )}
      <SectionContent sx={contentSx}>{children}</SectionContent>
    </SectionContainer>
  );
};

export default Section;
