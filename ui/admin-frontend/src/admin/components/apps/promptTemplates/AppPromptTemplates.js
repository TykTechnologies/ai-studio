import React, { useCallback, useEffect, useRef, useState } from "react";
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
import Section from "../../common/Section";
import ConfirmationDialog from "../../common/ConfirmationDialog";
import Can from "../../rbac/Can";
import { P } from "../../../rbac/permissions";
import { usePermissions } from "../../../context/PermissionsContext";
import { useEdition } from "../../../context/EditionContext";
import apiClient from "../../../utils/apiClient";
import { StyledTableCell, StyledTableHeaderCell, StyledTableRow } from "../../../styles/sharedStyles";
import PromptTemplateStatus from "./PromptTemplateStatus";
import PromptTemplateEditor from "./PromptTemplateEditor";
import PromptTemplateReview from "./PromptTemplateReview";
import { expiresAt, placementLabel } from "./promptTemplateText";
import {
  apiErrorMessage,
  deletePromptTemplate,
  listPromptTemplates,
  retirePromptTemplate,
} from "../../../services/promptTemplateService";

// Trusted prompt templates on the App page (features/TrustedPromptTemplates.md):
// the app's own prompt text, approved once by an administrator, which
// request guardrails then leave unscored so they score only the slots.

const formatTime = (s) => (s ? new Date(s).toLocaleString() : "-");

/** useUserNames resolves user ids to names, once each; "User #id" otherwise. */
const useUserNames = (ids) => {
  const [names, setNames] = useState({});
  const asked = useRef(new Set());
  const key = ids.filter(Boolean).sort().join(",");
  useEffect(() => {
    key
      .split(",")
      .filter(Boolean)
      .forEach((id) => {
        if (asked.current.has(id)) return;
        asked.current.add(id);
        apiClient
          .get(`/users/${id}`)
          .then((r) => {
            const n = r?.data?.data?.attributes?.name || r?.data?.data?.attributes?.email;
            if (n) setNames((prev) => ({ ...prev, [id]: n }));
          })
          .catch(() => {});
      });
  }, [key]);
  return useCallback((id) => (id ? names[String(id)] || `User #${id}` : "-"), [names]);
};

const AppPromptTemplates = ({ appId }) => {
  const { isFullAdmin } = usePermissions();
  const { isEnterprise } = useEdition();
  const [templates, setTemplates] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [notice, setNotice] = useState(null);
  const [showRetired, setShowRetired] = useState(false);
  const [editor, setEditor] = useState({ open: false, template: null });
  const [reviewing, setReviewing] = useState(null);
  const [deleting, setDeleting] = useState(null);

  const userName = useUserNames(templates.flatMap((t) => [String(t.approved_by || ""), String(t.created_by || "")]));

  const load = useCallback(async () => {
    try {
      setTemplates(await listPromptTemplates(appId));
      setError(null);
    } catch (e) {
      setError(apiErrorMessage(e, "Could not load the prompt templates"));
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

  const shown = templates.filter((t) => showRetired || t.status !== "retired");
  const retiredCount = templates.length - templates.filter((t) => t.status !== "retired").length;

  return (
    <Section
      title="Prompt templates"
      description="The app's own prompt text, approved once, so request guardrails score only the parts that vary. A control for false positives, not a security boundary."
      actions={
        <Can permission={P.APPS_WRITE}>
          <Button
            variant="outlined"
            size="small"
            startIcon={<AddIcon />}
            onClick={() => setEditor({ open: true, template: null })}
          >
            Propose template
          </Button>
        </Can>
      }
      data-testid="app-prompt-templates"
    >
      {!isEnterprise && (
        <Alert severity="info" sx={{ mb: 2 }}>
          Templates act through Enterprise guardrail providers. In the Community Edition they can be proposed and
          approved, and change nothing.
        </Alert>
      )}
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
      ) : templates.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          No prompt templates. Propose one to stop guardrails flagging this app's own instructions.
        </Typography>
      ) : (
        <>
          {retiredCount > 0 && (
            <FormControlLabel
              control={<Switch size="small" checked={showRetired} onChange={(e) => setShowRetired(e.target.checked)} />}
              label={`Show retired versions (${retiredCount})`}
              sx={{ mb: 1 }}
            />
          )}
          <TableContainer>
            <Table size="small" aria-label="Prompt templates">
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Version</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Status</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Placement</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Approved</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Last matched</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Expires</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {shown.map((t) => {
                  const expiry = expiresAt(t);
                  return (
                    <StyledTableRow key={t.id} data-testid={`prompt-template-${t.id}`}>
                      <StyledTableCell sx={{ fontFamily: "monospace" }}>{t.name}</StyledTableCell>
                      <StyledTableCell>{t.version}</StyledTableCell>
                      <StyledTableCell>
                        <PromptTemplateStatus status={t.status} />
                      </StyledTableCell>
                      <StyledTableCell>{placementLabel(t.placement)}</StyledTableCell>
                      <StyledTableCell>
                        {t.approved_at ? (
                          <>
                            {userName(t.approved_by)}
                            <Typography variant="caption" display="block" color="text.secondary">
                              {formatTime(t.approved_at)}
                            </Typography>
                          </>
                        ) : (
                          "-"
                        )}
                      </StyledTableCell>
                      <StyledTableCell>{t.last_matched_at ? formatTime(t.last_matched_at) : "Never"}</StyledTableCell>
                      <StyledTableCell>
                        {expiry ? formatTime(expiry) : `${t.expires_after_days} days without a match`}
                      </StyledTableCell>
                      <StyledTableCell align="right" sx={{ whiteSpace: "nowrap" }}>
                        <Button size="small" onClick={() => setReviewing(t)}>
                          {isFullAdmin && t.status === "proposed" ? "Review and approve" : "Review"}
                        </Button>
                        <Can permission={P.APPS_WRITE}>
                          <Button size="small" onClick={() => setEditor({ open: true, template: t })}>
                            Edit
                          </Button>
                          {t.status !== "retired" && (
                            <Button
                              size="small"
                              onClick={() =>
                                act(() => retirePromptTemplate(appId, t.id), `${t.ref || `${t.name}@${t.version}`} retired.`)
                              }
                            >
                              Retire
                            </Button>
                          )}
                        </Can>
                        <Can permission={P.APPS_DELETE}>
                          <Button size="small" color="error" onClick={() => setDeleting(t)}>
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
          <Typography variant="caption" color="text.secondary" component="div" sx={{ mt: 1 }}>
            Callers may name the template they used with the X-Tyk-Prompt-Template header (name@version). It only
            orders the search and never exempts anything by itself.
          </Typography>
        </>
      )}

      <PromptTemplateEditor
        open={editor.open}
        appId={appId}
        template={editor.template}
        onClose={() => setEditor({ open: false, template: null })}
        onSaved={(saved) => {
          setEditor({ open: false, template: null });
          setNotice(
            saved?.status === "proposed"
              ? `${saved.ref || saved.name} is proposed. An administrator must approve it before it has any effect.`
              : "Template saved."
          );
          load();
        }}
      />

      <PromptTemplateReview
        open={Boolean(reviewing)}
        appId={appId}
        template={reviewing}
        all={templates}
        canApprove={isFullAdmin}
        userName={userName}
        onClose={() => setReviewing(null)}
        onApproved={(approved) => {
          setReviewing(null);
          setNotice(`${approved?.ref || "The template"} is approved.`);
          load();
        }}
      />

      <ConfirmationDialog
        open={Boolean(deleting)}
        title="Delete prompt template version"
        message={
          deleting
            ? `Delete ${deleting.ref || `${deleting.name}@${deleting.version}`}? Guardrails then score the text it covered. Retiring keeps it for the record instead.`
            : ""
        }
        buttonLabel="Delete"
        onConfirm={() => {
          const t = deleting;
          setDeleting(null);
          act(() => deletePromptTemplate(appId, t.id), `${t.ref || `${t.name}@${t.version}`} deleted.`);
        }}
        onCancel={() => setDeleting(null)}
        iconName="hexagon-exclamation"
        iconColor="text.criticalDefault"
        titleColor="text.criticalDefault"
        backgroundColor="background.surfaceCriticalDefault"
        borderColor="border.criticalDefault"
        primaryButtonComponent="danger"
      />
    </Section>
  );
};

export default AppPromptTemplates;
