import React, { useEffect, useState } from "react";
import { Alert, Button, Dialog, DialogActions, DialogContent, DialogTitle, Typography } from "@mui/material";
import apiClient from "../../utils/apiClient";
import EmbedderFormFields from "./EmbedderFormFields";
import useEmbedderOptions from "./useEmbedderOptions";
import { apiErrorDetail, attributesFromDraft, emptyDraft, validateDraft } from "./embedderModel";

/**
 * Create an embedder without leaving the form that needs one (a data
 * source, a semantic router). `onCreated` receives the new JSON:API row.
 * `minPrivacy` pre-fills a standalone embedder's privacy level so it can
 * serve the caller's data.
 */
const EmbedderCreateDialog = ({ open, onClose, onCreated, minPrivacy = 0 }) => {
  const [draft, setDraft] = useState(emptyDraft);
  const [errors, setErrors] = useState({});
  const [saveError, setSaveError] = useState("");
  const [busy, setBusy] = useState(false);
  const { llms, vendors } = useEmbedderOptions(open);

  useEffect(() => {
    if (open) {
      setDraft({ ...emptyDraft(), privacy_score: minPrivacy });
      setErrors({});
      setSaveError("");
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
        data: { type: "embedders", attributes: attributesFromDraft(draft) },
      });
      onCreated(response.data.data);
    } catch (error) {
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
          LLM management → Embedders.
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
