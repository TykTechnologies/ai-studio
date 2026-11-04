import React from "react";
import { Box, Button, Typography } from "@mui/material";
import EventGuide from "./EventGuide";
import FlaggedText from "./FlaggedText";
import { softenTarget } from "./softenTarget";
import Can from "../rbac/Can";
import { P } from "../../rbac/permissions";

// EventDetails is a compliance event's expanded view: what happened and how
// to fix it, the soften action, the flagged text and the raw metadata. The
// Filter Events table shows it under a row, and a linked event (?event=)
// shows it open.
const EventDetails = ({ event, opinions, onSoften }) => {
  const target = softenTarget(event);
  return (
    <Box sx={{ px: 4, py: 2, bgcolor: "action.hover" }}>
      <EventGuide event={event} />
      {target && (
        <Can permission={P.APPS_WRITE}>
          <Button size="small" variant="outlined" sx={{ mb: 2 }} onClick={() => onSoften(target)} data-testid={`soften-${event.id}`}>
            Not a threat? Soften for this app
          </Button>
        </Can>
      )}
      <FlaggedText metadata={event.metadata} appId={event.app_id} eventType={event.event_type} opinions={opinions} />
      <Typography variant="caption" color="text.secondary">
        Metadata
      </Typography>
      <Box
        component="pre"
        sx={{
          m: 0,
          mt: 1,
          fontSize: "0.8rem",
          fontFamily: "monospace",
          whiteSpace: "pre-wrap",
          wordBreak: "break-all",
        }}
      >
        {JSON.stringify(event.metadata, null, 2)}
      </Box>
    </Box>
  );
};

export default EventDetails;
