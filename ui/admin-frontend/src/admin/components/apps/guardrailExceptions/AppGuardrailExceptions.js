import React, { useCallback, useEffect, useState } from "react";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  FormControlLabel,
  Switch,
  Table,
  TableBody,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@mui/material";
import AddIcon from "@mui/icons-material/Add";
import { Link as RouterLink } from "react-router-dom";
import SubSection from "../../common/SubSection";
import ConfirmationDialog from "../../common/ConfirmationDialog";
import Can from "../../rbac/Can";
import { P } from "../../../rbac/permissions";
import { usePermissions } from "../../../context/PermissionsContext";
import apiClient from "../../../utils/apiClient";
import { listAll } from "../../../utils/listAll";
import { StyledTableCell, StyledTableHeaderCell, StyledTableRow } from "../../../styles/sharedStyles";
import PromptTemplateStatus from "../promptTemplates/PromptTemplateStatus";
import { useUserNames } from "../promptTemplates/AppPromptTemplates";
import GuardrailExceptionEditor from "./GuardrailExceptionEditor";
import GuardrailExceptionReview from "./GuardrailExceptionReview";
import {
  apiErrorMessage,
  channelLabel,
  deleteGuardrailException,
  exceptionActionText,
  exceptionExpiresAt,
  listGuardrailExceptions,
  retireGuardrailException,
} from "../../../services/guardrailExceptionService";

// Guardrail exceptions on the App page (features/GuardrailExceptions.md):
// per filter, detector and channel, what a filter does instead of blocking
// this app's traffic, within what the app's attested capability allows.

const formatTime = (s) => (s ? new Date(s).toLocaleString() : "-");

/** useGuardrailFilters loads the guardrail filters, for names and the editor. */
const useGuardrailFilters = () => {
  const [filters, setFilters] = useState([]);
  useEffect(() => {
    let cancelled = false;
    listAll(apiClient, "/filters")
      .then((response) => {
        const rows = Array.isArray(response?.data) ? response.data : response?.data?.data || [];
        if (!cancelled) {
          setFilters(
            rows
              .filter((r) => r?.attributes?.kind === "guardrail")
              .map((r) => ({ id: Number(r.id), name: r.attributes?.name || `Filter #${r.id}` }))
          );
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);
  return filters;
};

const AppGuardrailExceptions = ({ appId, capability }) => {
  const { isFullAdmin } = usePermissions();
  const filters = useGuardrailFilters();
  const [exceptions, setExceptions] = useState([]);
  const [meta, setMeta] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState(null);
  const [showRetired, setShowRetired] = useState(false);
  const [editor, setEditor] = useState({ open: false, exception: null });
  const [reviewing, setReviewing] = useState(null);
  const [deleting, setDeleting] = useState(null);

  const userName = useUserNames(exceptions.flatMap((e) => [String(e.approved_by || ""), String(e.proposed_by || "")]));
  const filterName = (id) => filters.find((f) => f.id === Number(id))?.name || `Filter #${id}`;

  const load = useCallback(async () => {
    try {
      const { exceptions: list, meta: got } = await listGuardrailExceptions(appId);
      setExceptions(list);
      setMeta(got);
      setError(null);
      return list;
    } catch (e) {
      setError(apiErrorMessage(e, "Could not load the guardrail exceptions"));
    } finally {
      setLoading(false);
    }
  }, [appId]);

  useEffect(() => {
    load();
  }, [load]);

  const act = async (fn, done) => {
    setError(null);
    try {
      await fn();
      setNotice(done);
      await load();
    } catch (e) {
      setError(apiErrorMessage(e, "The action failed"));
    }
  };

  const shown = exceptions.filter((e) => showRetired || e.status !== "retired");
  const retiredCount = exceptions.filter((e) => e.status === "retired").length;
  const approvedCount = exceptions.filter((e) => e.status === "approved").length;

  return (
    <SubSection
      title="Exceptions"
      description="What a guardrail filter does instead of blocking this app's traffic, for one filter, detector and channel. Every hit is still recorded."
      actions={
        <Can permission={P.APPS_WRITE}>
          <Button
            variant="outlined"
            size="small"
            startIcon={<AddIcon />}
            disabled={filters.length === 0 || !meta}
            onClick={() => setEditor({ open: true, exception: null })}
          >
            {isFullAdmin ? "Add exception" : "Propose exception"}
          </Button>
        </Can>
      }
      data-testid="app-guardrail-exceptions"
    >
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}
      {notice && (
        <Alert severity="success" sx={{ mb: 2 }} onClose={() => setNotice(null)}>
          {notice}
        </Alert>
      )}
      {loading ? (
        <Box display="flex" justifyContent="center" p={2}>
          <CircularProgress size={24} />
        </Box>
      ) : exceptions.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          No exceptions. A false positive can be softened from its event in Compliance, or added here.
        </Typography>
      ) : (
        <>
          {retiredCount > 0 && (
            <FormControlLabel
              control={<Switch size="small" checked={showRetired} onChange={(e) => setShowRetired(e.target.checked)} />}
              label={`Show retired (${retiredCount})`}
              sx={{ mb: 1 }}
            />
          )}
          <TableContainer>
            <Table size="small" aria-label="Guardrail exceptions">
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>Filter</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Detector</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Where</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Instead of blocking</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Status</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Approved</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Hits</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Last hit</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Expires</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {shown.map((e) => {
                  const expiry = exceptionExpiresAt(e);
                  return (
                    <StyledTableRow key={e.id} data-testid={`guardrail-exception-${e.id}`}>
                      <StyledTableCell>
                        <RouterLink to={`/admin/filters/${e.filter_id}`}>{filterName(e.filter_id)}</RouterLink>
                      </StyledTableCell>
                      <StyledTableCell sx={{ fontFamily: "monospace" }}>{e.detector || "Any"}</StyledTableCell>
                      <StyledTableCell>
                        {channelLabel(e.channel)}
                        {e.tool_name ? ` (${e.tool_name})` : ""}
                      </StyledTableCell>
                      <StyledTableCell>{exceptionActionText(e)}</StyledTableCell>
                      <StyledTableCell>
                        <PromptTemplateStatus status={e.status} />
                      </StyledTableCell>
                      <StyledTableCell>
                        {e.approved_at ? (
                          <>
                            {userName(e.approved_by)}
                            <Typography variant="caption" display="block" color="text.secondary">
                              {formatTime(e.approved_at)}
                            </Typography>
                          </>
                        ) : (
                          "-"
                        )}
                      </StyledTableCell>
                      <StyledTableCell>{e.hit_count || 0}</StyledTableCell>
                      <StyledTableCell>{e.last_hit_at ? formatTime(e.last_hit_at) : "Never"}</StyledTableCell>
                      <StyledTableCell>
                        {expiry ? formatTime(expiry) : `${e.expires_after_days} days without a hit`}
                      </StyledTableCell>
                      <StyledTableCell align="right" sx={{ whiteSpace: "nowrap" }}>
                        <Button size="small" onClick={() => setReviewing(e)}>
                          {isFullAdmin && e.status === "proposed" ? "Review and approve" : "Review"}
                        </Button>
                        {e.status !== "retired" && (
                          <Can permission={P.APPS_WRITE}>
                            <Button size="small" onClick={() => setEditor({ open: true, exception: e })}>
                              Edit
                            </Button>
                            <Button
                              size="small"
                              onClick={() => act(() => retireGuardrailException(appId, e.id), `Exception #${e.id} retired.`)}
                            >
                              Retire
                            </Button>
                          </Can>
                        )}
                        <Can permission={P.APPS_DELETE}>
                          <Button size="small" color="error" onClick={() => setDeleting(e)}>
                            Delete
                          </Button>
                        </Can>
                      </StyledTableCell>
                    </StyledTableRow>
                  );
                })}
              </TableBody>
            </Table>
          </TableContainer>
          {meta?.max_approved ? (
            <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 1 }}>
              {approvedCount} of at most {meta.max_approved} approved. An approved exception lapses back to proposed
              when it has not applied for its expiry period.
            </Typography>
          ) : null}
        </>
      )}

      <GuardrailExceptionEditor
        open={editor.open}
        appId={appId}
        exception={editor.exception}
        filters={filters}
        meta={meta}
        capability={capability}
        isFullAdmin={isFullAdmin}
        onClose={() => setEditor({ open: false, exception: null })}
        onSaved={(saved) => {
          setEditor({ open: false, exception: null });
          setNotice(
            saved?.status === "proposed"
              ? `Exception #${saved.id} is proposed. An administrator must approve it before it has any effect.`
              : `Exception #${saved?.id} saved.`
          );
          load();
        }}
      />

      <GuardrailExceptionReview
        open={Boolean(reviewing)}
        appId={appId}
        exception={reviewing}
        filterName={filterName}
        canApprove={isFullAdmin}
        userName={userName}
        onClose={() => setReviewing(null)}
        onReload={async () => {
          const list = await load();
          const id = reviewing?.id;
          setReviewing((list || []).find((e) => e.id === id) || null);
        }}
        onApproved={(approved) => {
          setReviewing(null);
          setNotice(`Exception #${approved?.id} is approved.`);
          load();
        }}
      />

      <ConfirmationDialog
        open={Boolean(deleting)}
        title="Delete guardrail exception"
        message={
          deleting
            ? `Delete exception #${deleting.id}? The filter then acts on this traffic as configured. Retiring keeps it for the record instead.`
            : ""
        }
        buttonLabel="Delete"
        onConfirm={() => {
          const e = deleting;
          setDeleting(null);
          act(() => deleteGuardrailException(appId, e.id), `Exception #${e.id} deleted.`);
        }}
        onCancel={() => setDeleting(null)}
        iconName="hexagon-exclamation"
        iconColor="text.criticalDefault"
        titleColor="text.criticalDefault"
        backgroundColor="background.surfaceCriticalDefault"
      />
    </SubSection>
  );
};

export default AppGuardrailExceptions;
