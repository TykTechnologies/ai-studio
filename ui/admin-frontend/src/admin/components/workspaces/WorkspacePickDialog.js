import React, { useEffect, useState } from "react";
import { Button, Dialog, DialogActions, DialogContent, DialogTitle, Typography } from "@mui/material";
import WorkspaceSelect from "./WorkspaceSelect";

/**
 * WorkspacePickDialog asks where a new object should land, for actions that
 * create one without a form (clone). It offers the Workspaces where the
 * caller holds `permission`; onConfirm receives the chosen id. `error`
 * shows a server refusal (workspace_required, invalid_workspace) on the
 * field. Callers open it only when there is a choice (useCanMove).
 */
const WorkspacePickDialog = ({ open, permission, title, description, confirmLabel = "Continue", error = "", busy = false, onConfirm, onCancel }) => {
  const [workspaceId, setWorkspaceId] = useState(null);

  useEffect(() => {
    if (open) setWorkspaceId(null);
  }, [open]);

  return (
    <Dialog open={open} onClose={busy ? undefined : onCancel} fullWidth maxWidth="xs" data-testid="workspace-pick-dialog">
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        {description && (
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            {description}
          </Typography>
        )}
        <WorkspaceSelect permission={permission} value={workspaceId} onChange={setWorkspaceId} error={error} />
      </DialogContent>
      <DialogActions>
        <Button onClick={onCancel} disabled={busy}>
          Cancel
        </Button>
        <Button variant="contained" onClick={() => onConfirm(workspaceId)} disabled={busy || workspaceId === null}>
          {confirmLabel}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default WorkspacePickDialog;
