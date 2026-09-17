import React from "react";
import { Box, IconButton, Tooltip, Typography } from "@mui/material";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";

const copyText = (value) => {
  if (navigator.clipboard?.writeText) {
    navigator.clipboard.writeText(value).catch(() => undefined);
  }
};

/** A one-line value (a URL, a header) with a copy button. */
const CopyableCode = ({ value, label, testId }) => (
  <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
    <Typography
      component="code"
      variant="body2"
      data-testid={testId}
      sx={{
        fontFamily: "monospace",
        bgcolor: "action.hover",
        px: 1.5,
        py: 1,
        borderRadius: 1,
        flexGrow: 1,
        wordBreak: "break-all",
      }}
    >
      {value}
    </Typography>
    <Tooltip title={`Copy ${label}`}>
      <IconButton aria-label={`Copy ${label}`} size="small" onClick={() => copyText(value)}>
        <ContentCopyIcon fontSize="small" />
      </IconButton>
    </Tooltip>
  </Box>
);

/** A multi-line block (a config file, a code sample) with a copy button. */
export const CopyableBlock = ({ value, label, testId }) => (
  <Box sx={{ position: "relative" }}>
    <Box
      component="pre"
      data-testid={testId}
      sx={{
        fontFamily: "monospace",
        fontSize: "0.8rem",
        bgcolor: "action.hover",
        borderRadius: 1,
        p: 1.5,
        pr: 5,
        m: 0,
        overflowX: "auto",
        whiteSpace: "pre",
      }}
    >
      {value}
    </Box>
    <Tooltip title={`Copy ${label}`}>
      <IconButton
        aria-label={`Copy ${label}`}
        size="small"
        onClick={() => copyText(value)}
        sx={{ position: "absolute", top: 4, right: 4 }}
      >
        <ContentCopyIcon fontSize="small" />
      </IconButton>
    </Tooltip>
  </Box>
);

export default CopyableCode;
