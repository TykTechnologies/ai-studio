import React from "react";
import { Box, Typography } from "@mui/material";
import { eventGuide } from "./guardrailTriage";

const List = ({ items, testId }) => (
  <Box component="ul" sx={{ m: 0, mt: 0.25, pl: 2.5 }} data-testid={testId}>
    {items.map((f) => (
      <Typography key={f} component="li" variant="body2">
        {f}
      </Typography>
    ))}
  </Box>
);

// The headline, the reasons and the fixes for a guardrail compliance event.
const EventGuide = ({ event }) => {
  const g = eventGuide(event?.event_type, event?.metadata, event?.filter_name);
  if (!g) return null;
  return (
    <Box sx={{ mb: 2 }} data-testid="event-guide">
      <Typography variant="subtitle2" data-testid="event-guide-headline">
        {g.headline}
      </Typography>
      {g.code && (
        <Typography variant="caption" color="text.secondary" component="div">
          Caller got <Box component="span" sx={{ fontFamily: "monospace" }}>{g.code}</Box>
        </Typography>
      )}
      <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 1 }}>
        Why
      </Typography>
      <List items={g.why} testId="event-guide-why" />
      <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 1 }}>
        Fix
      </Typography>
      <List items={g.fix} testId="event-guide-fix" />
    </Box>
  );
};

export default EventGuide;
