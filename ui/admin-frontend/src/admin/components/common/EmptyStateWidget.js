import React from "react";
import { Paper, Typography, Box } from "@mui/material";
import { alpha } from "@mui/material/styles";
import { PrimaryButton } from "../../styles/sharedStyles";

const EmptyStateWidget = ({
  title,
  description,
  buttonText,
  buttonIcon,
  onButtonClick,
}) => (
  <Paper
    elevation={3}
    sx={{
      p: 4,
      boxShadow: 0,
      textAlign: "center",
      backgroundColor: (theme) =>
        alpha(theme.palette.custom.emptyStateBackground, 0.1),
      border: (theme) => `1px solid ${alpha(theme.palette.info.main, 0.2)}`,
    }}
  >
    <Typography variant="h6" gutterBottom>
      {title}
    </Typography>
    <Typography variant="body1" paragraph>
      {description}
    </Typography>
    <Box mt={2}>
      <PrimaryButton
        variant="contained"
        startIcon={buttonIcon}
        onClick={onButtonClick}
      >
        {buttonText}
      </PrimaryButton>
    </Box>
  </Paper>
);

export default EmptyStateWidget;
