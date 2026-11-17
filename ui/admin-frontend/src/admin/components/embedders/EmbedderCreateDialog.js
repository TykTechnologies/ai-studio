import React, { useEffect, useState } from "react";
import { Alert, Box, Button, Dialog, DialogActions, DialogContent, DialogTitle, Typography } from "@mui/material";
import apiClient from "../../utils/apiClient";
import EmbedderFormFields from "./EmbedderFormFields";
import useEmbedderOptions from "./useEmbedderOptions";
import { apiErrorDetail, attributesFromDraft, emptyDraft, validateDraft } from "./embedderModel";
import WorkspaceSelect from "../workspaces/WorkspaceSelect";
import { workspaceFieldError } from "../workspaces/workspaceFields";
import { P } from "../../rbac/permissions";

/**
 * Create an embedder without leaving the form that needs one (a data
 * source, a semantic router). `onCreated` receives the new JSON:API row.
 * `minPrivacy` pre-fills a standalone embedder's privacy level so it can
 * serve the caller's data. In Enterprise the embedder lands in the
 * Workspace chosen here (preselected when the caller has one choice).
 */
const EmbedderCreateDialog = ({ open, onClose, onCreated, minPrivacy = 0 }) => {
  const [draft, setDraft] = useState(emptyDraft);
  const [errors, setErrors] = useState({});
  const [saveError, setSaveError] = useState("");
  const [busy, setBusy] = useState(false);
  const [workspaceId, setWorkspaceId] = useState(null);
  const [workspaceError, setWorkspaceError] = useState("");
  const { llms, vendors } = useEmbedderOptions(open);

  useEffect(() => {
    if (open) {
      setDraft({ ...emptyDraft(), privacy_score: minPrivacy });
      setErrors({});
      setSaveError("");
      setWorkspaceError("");
    }
  }, [open, minPrivacy]);

  const submit = async () => {
    const found = validateDraft(draft);
    setErrors(found);
    if (Object.keys(found).length > 0) return;
    setBusy(true);
    setSaveError("");
    try {
      const response = await apiClient.post("/embedders", {
        data: {
          type: "embedders",
          attributes: {
            ...attributesFromDraft(draft),
            ...(workspaceId !== null && workspaceId !== undefined ? { workspace_id: Number(workspaceId) } : {}),
          },
        },
      });
      onCreated(response.data.data);
    } catch (error) {
      const wsError = workspaceFieldError(error);
      if (wsError) {
        setWorkspaceError(wsError);
        return;
      }
      setSaveError(apiErrorDetail(error, "Creating the embedder failed"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onClose={busy ? undefined : onClose} fullWidth maxWidth="md" data-testid="embedder-create-dialog">
      <DialogTitle>New embedder</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          An embedder turns text into vectors. Once saved it can be reused by other data sources and managed under
          Model management → Embedders.
        </Typography>
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
          idPrefix="embedder-dialog"
        />
        <Box sx={{ mt: 2 }}>
          <WorkspaceSelect
            permission={P.EMBEDDERS_WRITE}
            value={workspaceId}
            onChange={(ws) => {
              setWorkspaceId(ws);
              setWorkspaceError("");
            }}
            error={workspaceError}
            helperText="The Workspace the embedder is created in."
            name="embedder_workspace_id"
          />
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={busy}>
          Cancel
        </Button>
        <Button variant="contained" onClick={submit} disabled={busy}>
          Create embedder
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default EmbedderCreateDialog;
