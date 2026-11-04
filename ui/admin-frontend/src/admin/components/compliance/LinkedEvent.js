import React, { useEffect, useRef, useState } from "react";
import { Alert, Box, Button, Chip, CircularProgress, Link, Typography } from "@mui/material";
import { Link as RouterLink } from "react-router-dom";
import { StyledPaper } from "../../styles/sharedStyles";
import apiClient from "../../utils/apiClient";
import { eventSourceLabel } from "./eventSource";
import EventDetails from "./EventDetails";
import { isPermissionDenied } from "../../utils/apiErrors";

const SEVERITY_COLORS = { critical: "error", warning: "warning", info: "info" };

// LinkedEvent shows the one compliance event a link names
// (/admin/compliance?event=<id>, the link guardrail alerts and outcome
// events carry), open, whatever the page's period. onClose drops the link.
const LinkedEvent = ({ eventId, opinions, onSoften, onClose }) => {
  const [event, setEvent] = useState(null);
  const [status, setStatus] = useState("loading");
  const ref = useRef(null);

  useEffect(() => {
    let cancelled = false;
    setStatus("loading");
    setEvent(null);
    apiClient
      .get(`/compliance/events/${encodeURIComponent(eventId)}`)
      .then((response) => {
        if (cancelled) return;
        setEvent(response.data);
        setStatus("ok");
      })
      .catch((err) => {
        if (cancelled) return;
        const code = err?.response?.status;
        if (code === 404 || code === 400) setStatus("missing");
        else if (isPermissionDenied(err)) setStatus("denied");
        else setStatus("error");
      });
    return () => {
      cancelled = true;
    };
  }, [eventId]);

  useEffect(() => {
    if (status === "ok" && ref.current && typeof ref.current.scrollIntoView === "function") {
      ref.current.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }, [status]);

  const close = (
    <Button size="small" onClick={onClose} data-testid="linked-event-close">
      Show all events
    </Button>
  );

  if (status === "loading") {
    return (
      <Box sx={{ display: "flex", justifyContent: "center", py: 2 }} data-testid="linked-event-loading">
        <CircularProgress size={24} />
      </Box>
    );
  }
  if (status !== "ok") {
    const message = {
      missing: `Compliance event ${eventId} was not found. It may have been removed by data retention.`,
      denied: "You do not have permission to view this compliance event.",
      error: `Compliance event ${eventId} could not be loaded.`,
    }[status];
    return (
      <Alert severity={status === "missing" ? "warning" : "error"} sx={{ mb: 3 }} action={close} data-testid="linked-event-error">
        {message}
      </Alert>
    );
  }

  return (
    <StyledPaper ref={ref} sx={{ mb: 3, border: 2, borderColor: "primary.main" }} data-testid="linked-event">
      <Box sx={{ p: 2, display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
        <Typography variant="subtitle1" sx={{ fontWeight: 500, mr: 1 }}>
          Linked event #{event.id}
        </Typography>
        <Chip label={event.event_type} size="small" variant="outlined" />
        <Chip label={event.severity} size="small" color={SEVERITY_COLORS[event.severity] || "default"} />
        <Box sx={{ flexGrow: 1 }} />
        {close}
      </Box>
      <Box sx={{ px: 2, pb: 2 }}>
        <Typography variant="body2" color="text.secondary">
          {new Date(event.timestamp).toLocaleString()}
          {" · "}
          {event.app_id ? (
            <Link component={RouterLink} to={`/admin/apps/${event.app_id}`}>
              {event.app_name || `App ${event.app_id}`}
            </Link>
          ) : (
            "no app"
          )}
          {` · ${event.filter_name || "-"}`}
          {eventSourceLabel(event.metadata) ? ` · ${eventSourceLabel(event.metadata)}` : ""}
        </Typography>
        {event.description && (
          <Typography variant="body2" sx={{ mt: 1 }} data-testid="linked-event-description">
            {event.description}
          </Typography>
        )}
      </Box>
      {event.metadata && Object.keys(event.metadata).length > 0 && <EventDetails event={event} opinions={opinions} onSoften={onSoften} />}
    </StyledPaper>
  );
};

export default LinkedEvent;
