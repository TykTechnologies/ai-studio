import React, { useState } from "react";
import { Link as RouterLink, useParams } from "react-router-dom";
import { Alert, Box, LinearProgress, Snackbar, Tab, Tabs, Typography } from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import { TitleBox, ContentBox, SecondaryLinkButton } from "../styles/sharedStyles";
import EnterpriseFeatureBadge from "../components/common/EnterpriseFeatureBadge";
import PermissionDeniedPanel from "../components/rbac/PermissionDeniedPanel";
import { SessionDetailProvider, useSessionDetail } from "../components/agent-tracing/SessionDetailContext";
import useSessionData from "../components/agent-tracing/useSessionData";
import useSessionKeyboard from "../components/agent-tracing/useSessionKeyboard";
import { SessionHeader, SummaryStrip } from "../components/agent-tracing/SessionHeader";
import TimelineStrip from "../components/agent-tracing/TimelineStrip";
import SpanDrawer from "../components/agent-tracing/SpanDrawer";
import FindBar from "../components/agent-tracing/FindBar";
import TranscriptView from "../components/agent-tracing/TranscriptView";
import TimelineView from "../components/agent-tracing/TimelineView";
import { useEdition } from "../context/EditionContext";
import { isEnterpriseFeature, isPermissionDenied } from "../utils/apiErrors";
import { P } from "../rbac/permissions";

// Agent session detail (spec §6.2; Enterprise): header, summary strip,
// timeline strip, the main views (keys 1/2) and the span drawer, all on one
// shared selection model and time range.

// View registry: Transcript (jyx.30) and Timeline (jyx.31) plug in here;
// Tree (key 3) is Phase 2 and has no tab yet.
export const VIEWS = [
  { value: "transcript", label: "Transcript", key: "1", Component: TranscriptView },
  { value: "timeline", label: "Timeline", key: "2", Component: TimelineView },
];

const Body = ({ loadingSpans }) => {
  const { view, dispatch } = useSessionDetail();
  const [toast, setToast] = useState(null);
  useSessionKeyboard();
  const current = VIEWS.find((v) => v.value === view) || VIEWS[0];
  const View = current.Component;
  return (
    <>
      <SessionHeader onNotify={(message, severity) => setToast({ message, severity })} />
      <SummaryStrip />
      <TimelineStrip />
      <FindBar />
      <Tabs value={current.value} onChange={(_, v) => dispatch({ type: "view", view: v })} aria-label="Session views" sx={{ mb: 1 }}>
        {VIEWS.map((v) => (
          <Tab key={v.value} value={v.value} label={`${v.label} (${v.key})`} id={`view-tab-${v.value}`} />
        ))}
      </Tabs>
      {loadingSpans && <LinearProgress aria-label="Loading spans" sx={{ mb: 1 }} />}
      <Box role="tabpanel" aria-labelledby={`view-tab-${current.value}`}>
        <View />
      </Box>
      <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: 1 }}>
        Keys: j/k steps · n/N tool calls · e/E errors · g gateway events · / find · Enter open · Esc close
      </Typography>
      <SpanDrawer />
      <Snackbar open={Boolean(toast)} autoHideDuration={4000} onClose={() => setToast(null)}>
        {toast ? (
          <Alert severity={toast.severity} onClose={() => setToast(null)}>
            {toast.message}
          </Alert>
        ) : (
          <span />
        )}
      </Snackbar>
    </>
  );
};

const AgentSessionDetail = () => {
  const { sessionId } = useParams();
  const { isEnterprise } = useEdition();
  const title = (
    <TitleBox top="var(--studio-header-height)">
      {/* The page title never carries the session name or external id. */}
      <Typography variant="headingXLarge">Agent session</Typography>
      <SecondaryLinkButton component={RouterLink} to="/admin/agent-sessions" startIcon={<ArrowBackIcon />}>
        Back to sessions
      </SecondaryLinkButton>
    </TitleBox>
  );
  if (!isEnterprise) {
    return (
      <>
        {title}
        <ContentBox>
          <EnterpriseFeatureBadge feature="Agent Call Tracing" />
        </ContentBox>
      </>
    );
  }
  return <Loaded sessionId={sessionId} title={title} />;
};

const Loaded = ({ sessionId, title }) => {
  const { detail, spans, resources, previews, loadingSpans, error } = useSessionData(sessionId);
  if (error && isPermissionDenied(error)) return <PermissionDeniedPanel required={P.TRACES_READ} />;
  return (
    <>
      {title}
      <ContentBox>
        {error && isEnterpriseFeature(error) && <EnterpriseFeatureBadge feature="Agent Call Tracing" />}
        {error && !isEnterpriseFeature(error) && (
          <Alert severity="error">
            {error?.response?.status === 404 ? "This session does not exist or has been purged." : "Could not load this session."}
          </Alert>
        )}
        {!detail && !error && <LinearProgress aria-label="Loading session" />}
        {detail && (
          <SessionDetailProvider detail={detail} spans={spans} resources={resources} previews={previews}>
            <Body loadingSpans={loadingSpans} />
          </SessionDetailProvider>
        )}
      </ContentBox>
    </>
  );
};

export default AgentSessionDetail;
