import React from "react";
import { Box, Typography } from "@mui/material";
import { eventGuide } from "./guardrailTriage";

// "What happened" and "How to fix" for a guardrail compliance event.
const EventGuide = ({ event }) => {
  const g = eventGuide(event?.event_type, event?.metadata, event?.filter_name);
  if (!g) return null;
  return (
    <Box sx={{ mb: 2 }} data-testid="event-guide">
      <Typography variant="caption" color="text.secondary">
        What happened
      </Typography>
      <Typography variant="body2" sx={{ mt: 0.5 }} data-testid="event-guide-what">
        {g.what}
      </Typography>
      {g.code && (
        <Typography variant="caption" color="text.secondary" component="div">
          The caller got reason code <Box component="span" sx={{ fontFamily: "monospace" }}>{g.code}</Box>.
        </Typography>
      )}
      <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 1 }}>
        How to fix
      </Typography>
      <Box component="ul" sx={{ m: 0, mt: 0.5, pl: 2.5 }} data-testid="event-guide-fix">
        {g.fix.map((f) => (
          <Typography key={f} component="li" variant="body2">
            {f}
          </Typography>
        ))}
      </Box>
    </Box>
  );
};

export default EventGuide;
