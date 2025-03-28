import React from "react";
import { Paper, Typography, Box, Link } from "@mui/material";
import OpenInNewIcon from "@mui/icons-material/OpenInNew";

const EmptyStateWidget = ({
  title,
  description,
  learnMoreLink,
}) => (
  <Paper
    sx={{
      p: 2,
      boxShadow: 0,
      textAlign: "center",
      border: (theme) => `1px solid ${theme.palette.border.neutralDefault}`,
    }}
  >
    <Box sx={{ mb: 2, display: 'flex', justifyContent: 'center', p: 2 }}>
      <img
        src="/empty-state.png"
        alt="Empty state illustration"
        style={{
          maxWidth: "50%",
        }}
      />
    </Box>
    
    <Box sx={{ mb: 1, px: 5, lineHeight: 3 }}>
      <Typography variant="headingLarge" color="text.primary" gutterBottom>
        {title}
      </Typography>
      <Typography variant="bodyLargeDefault" color="text.defaultSubdued" paragraph>
        {description}
      </Typography>
      <Box mt={2} display="flex" justifyContent="center" alignItems="center">
        <Link
          variant="bodyLargeMedium"
          color="text.linkDefault"
          href={learnMoreLink || "#"}
          onClick={(e) => !learnMoreLink && e.preventDefault()}
          sx={{
            display: "flex",
            alignItems: "center",
            textDecoration: "none"
          }}
        >
          Learn more
          <OpenInNewIcon sx={{ ml: 0.5, color: "inherit", width:"14px", height:"14px" }} />
        </Link>
      </Box>
    </Box>
  </Paper>
);

export default EmptyStateWidget;
