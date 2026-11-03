import React from "react";
import { Box, Typography } from "@mui/material";

/**
 * A titled part of a Section: the same header (title, one-line description,
 * actions on the right) one level down, for a Section that groups several
 * blocks (the App page's Guardrails).
 */
const SubSection = ({ title, description, actions, children, sx = {}, ...rest }) => (
  <Box sx={{ "& + &": { mt: 3 }, ...sx }} {...rest}>
    {(title || actions) && (
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 2, mb: 1 }}>
        <Box sx={{ minWidth: 0 }}>
          {title && (
            <Typography variant="subtitle1" component="h3" sx={{ fontWeight: 600 }}>
              {title}
            </Typography>
          )}
          {description && (
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.25 }}>
              {description}
            </Typography>
          )}
        </Box>
        {actions && <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexShrink: 0 }}>{actions}</Box>}
      </Box>
    )}
    {children}
  </Box>
);

export default SubSection;
