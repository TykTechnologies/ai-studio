import React, { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { Alert, Box, Typography } from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import apiClient from "../../utils/apiClient";
import { ContentBox, PrimaryButton, SecondaryLinkButton, SecondaryOutlineButton, TitleBox } from "../../styles/sharedStyles";
import { useConfirmNavigation, useUnsavedForm } from "../../../components/unsaved-changes";
import { fetchDependents } from "../../utils/dependentsMessage";
import EmbedderFormFields from "./EmbedderFormFields";
import useEmbedderOptions from "./useEmbedderOptions";
import { apiErrorDetail, attributesFromDraft, draftFromAttributes, emptyDraft, validateDraft } from "./embedderModel";
import { P } from "../../rbac/permissions";
import WorkspaceSelect from "../workspaces/WorkspaceSelect";
import { useCanMove, workspaceFieldError } from "../workspaces/workspaceFields";

// Why the model and API compatibility are read-only while data sources use
// the embedder; the server refuses the change too (409).
export const lockedReasonFor = (dependents) => {
  const names = (dependents?.datasources || []).map((d) => d.name);
  if (names.length === 0) return "";
  return (
    `Used by ${names.join(", ")}. Their stored vectors came from this model, so the model and API compatibility ` +
    "cannot change. To switch model, create a new embedder, point the data sources at it and re-process their embeddings."
  );
};

const EmbedderForm = () => {
  const { id } = useParams();
  const isEdit = Boolean(id);
  const navigate = useNavigate();
  const [draft, setDraft] = useState(emptyDraft);
  const [errors, setErrors] = useState({});
  const [saveError, setSaveError] = useState("");
  const [lockedReason, setLockedReason] = useState("");
  const [loaded, setLoaded] = useState(!isEdit);
  const { llms, vendors } = useEmbedderOptions();
  // Enterprise: the Workspace it lives in (landing rule when null), and the
  // stored one of the embedder being edited: a different one on save is a move.
  const [workspaceId, setWorkspaceId] = useState(null);
  const [originalWorkspaceId, setOriginalWorkspaceId] = useState(null);
  const [workspaceError, setWorkspaceError] = useState("");
  const canMove = useCanMove(P.EMBEDDERS_WRITE);

  const { markSaved } = useUnsavedForm({ ...draft, workspace_id: workspaceId }, { ready: loaded });
  const confirmNavigation = useConfirmNavigation();

  useEffect(() => {
    if (!isEdit) return;
    apiClient
      .get(`/embedders/${id}`)
      .then((response) => {
        const attrs = response.data.data.attributes;
        setDraft(draftFromAttributes(attrs));
        setWorkspaceId(attrs.workspace_id ?? null);
        setOriginalWorkspaceId(attrs.workspace_id ?? null);
        setLoaded(true);
      })
      .catch(() => setSaveError("Failed to load embedder"));
    fetchDependents("embedders", id).then((deps) => setLockedReason(lockedReasonFor(deps)));
  }, [id, isEdit]);

  const handleSubmit = async () => {
    const found = validateDraft(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) return;
    setSaveError("");
    // workspace_id is sent only to choose one (create) or to move (edit).
    const workspaceAttr =
      workspaceId !== null && workspaceId !== undefined && (!isEdit || Number(workspaceId) !== Number(originalWorkspaceId))
        ? { workspace_id: Number(workspaceId) }
        : {};
    const body = { data: { type: "embedders", attributes: { ...attributesFromDraft(draft), ...workspaceAttr } } };
    try {
      const response = isEdit
        ? await apiClient.patch(`/embedders/${id}`, body)
        : await apiClient.post("/embedders", body);
      markSaved();
      navigate(`/admin/embedders/${response.data.data.id}`);
    } catch (error) {
      const wsError = workspaceFieldError(error);
      if (wsError) {
        setWorkspaceError(wsError);
        setSaveError(wsError);
        return;
      }
      setSaveError(apiErrorDetail(error, "Failed to save embedder"));
    }
  };

  return (
    <>
      <TitleBox top="var(--studio-header-height)">
        <Typography variant="headingXLarge">{isEdit ? "Edit embedder" : "Add embedder"}</Typography>
        <SecondaryLinkButton component={Link} to="/admin/embedders" startIcon={<ArrowBackIcon />} color="inherit">
          Back to embedders
        </SecondaryLinkButton>
      </TitleBox>
      <ContentBox>
        {saveError && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {saveError}
          </Alert>
        )}
        <EmbedderFormFields
          draft={draft}
          onChange={setDraft}
          errors={errors}
          llms={llms}
          vendors={vendors}
          lockedReason={lockedReason}
        />
        {/* Workspace (Enterprise): where it is created; on edit, the move control. */}
        <Box sx={{ mt: 3 }}>
          <WorkspaceSelect
            permission={P.EMBEDDERS_WRITE}
            value={workspaceId}
            onChange={(ws) => {
              setWorkspaceId(ws);
              setWorkspaceError("");
            }}
            disabled={isEdit && !canMove}
            error={workspaceError}
            helperText={isEdit ? "Moving it to another Workspace needs write in both." : "The Workspace it is created in."}
          />
        </Box>
        <Box sx={{ display: "flex", gap: 2, mt: 3 }}>
          <PrimaryButton variant="contained" onClick={handleSubmit}>
            {isEdit ? "Save embedder" : "Create embedder"}
          </PrimaryButton>
          <SecondaryOutlineButton onClick={() => confirmNavigation(() => navigate("/admin/embedders"))}>
            Cancel
          </SecondaryOutlineButton>
        </Box>
      </ContentBox>
    </>
  );
};

export default EmbedderForm;
