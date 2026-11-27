import React, { useState } from "react";
import { Link as RouterLink, useNavigate } from "react-router-dom";
import { Box, Link, Typography } from "@mui/material";
import apiClient from "../../utils/apiClient";
import { usePermissions } from "../../context/PermissionsContext";
import { P } from "../../rbac/permissions";
import { useSessionDetail } from "./SessionDetailContext";
import { attrsOf, gatewayEventOf } from "./spanModel";

// Gateway events whose findings are compliance events.
const LINKED_TYPES = new Set(["guardrail", "filter"]);

const REQUEST_ID = /^tyai_[0-9a-f]{32}$/;

export const complianceEventPath = (id) => `/admin/compliance?event=${encodeURIComponent(id)}`;

/**
 * requestIdOf is the gateway request id a guardrail or filter event's
 * compliance events record (metadata.request_id): the tyk.request.id of the
 * LLM span the event belongs to (tkg.2). "" when there is none.
 */
export const requestIdOf = (span, index) => {
  if (!LINKED_TYPES.has(gatewayEventOf(span).type)) return "";
  const parent = span?.parent_span_id ? index?.byId?.get(span.parent_span_id)?.span : null;
  const id = attrsOf(parent)["tyk.request.id"];
  return typeof id === "string" && REQUEST_ID.test(id) ? id : "";
};

const RequestIdText = ({ id }) => (
  <Typography
    component="span"
    variant="caption"
    color="text.secondary"
    title="Gateway request id (X-Tyk-AI-Request-ID)"
    sx={{ fontFamily: "monospace", userSelect: "all" }}
    data-testid="request-id"
  >
    {id}
  </Typography>
);

/**
 * ComplianceEventLink leads from a guardrail or filter event on the trace to
 * its compliance events on Filter Events (tkg.2): one opens it, several are
 * listed. A viewer without compliance read access, or an edition without
 * compliance events, sees the request id as text to search for instead.
 */
const ComplianceEventLink = ({ span }) => {
  const { index, appId } = useSessionDetail();
  const { can } = usePermissions();
  const id = requestIdOf(span, index);
  if (!id) return null;
  if (!can(P.COMPLIANCE_READ) || !appId) return <RequestIdText id={id} />;
  return <Lookup id={id} appId={appId} at={span.start_time} />;
};

const Lookup = ({ id, appId, at }) => {
  const navigate = useNavigate();
  const [state, setState] = useState({ status: "idle", events: [] });
  if (state.status === "unavailable") return <RequestIdText id={id} />;

  const find = async (e) => {
    e.preventDefault();
    e.stopPropagation();
    setState({ status: "loading", events: [] });
    try {
      const res = await apiClient.get("/compliance/events-by-request", { params: { request_id: id, app_id: appId, at } });
      const events = Array.isArray(res.data?.events) ? res.data.events : [];
      if (events.length === 1) {
        navigate(complianceEventPath(events[0].id));
        return;
      }
      setState({ status: events.length ? "several" : "none", events });
    } catch (err) {
      // 403 (no access, or no compliance events in this edition) or 404:
      // the id is what is left to go on.
      const code = err?.response?.status;
      setState({ status: code === 403 || code === 404 ? "unavailable" : "error", events: [] });
    }
  };

  return (
    <Box component="span" sx={{ display: "inline-flex", flexWrap: "wrap", gap: 0.5, alignItems: "center" }} onClick={(e) => e.stopPropagation()}>
      <Link component="button" variant="caption" onClick={find} disabled={state.status === "loading"} data-testid="compliance-event-link">
        {state.status === "loading" ? "Finding compliance event…" : "Compliance event"}
      </Link>
      {state.status === "none" && (
        <Typography component="span" variant="caption" color="text.secondary" data-testid="compliance-event-none">
          none found for <RequestIdText id={id} />
        </Typography>
      )}
      {state.status === "error" && (
        <Typography component="span" variant="caption" color="error" data-testid="compliance-event-error">
          could not look it up; request <RequestIdText id={id} />
        </Typography>
      )}
      {state.status === "several" &&
        state.events.map((ev) => (
          <Link key={ev.id} component={RouterLink} to={complianceEventPath(ev.id)} variant="caption" data-testid="compliance-event-item">
            {ev.event_type}
          </Link>
        ))}
    </Box>
  );
};

export default ComplianceEventLink;
