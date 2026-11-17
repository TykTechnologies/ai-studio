import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Alert, Box, Chip, Stack, Tab, Tabs, Tooltip, Typography } from '@mui/material';
import { TitleBox, ContentBox, PrimaryOutlineButton, DangerOutlineButton } from '../../styles/sharedStyles';
import ConfirmationDialog from '../../components/common/ConfirmationDialog';
import WorkspaceAccessTab from '../../components/workspaces/WorkspaceAccessTab';
import WorkspaceObjectsTab from '../../components/workspaces/WorkspaceObjectsTab';
import { usePermissions } from '../../context/PermissionsContext';
import { P } from '../../rbac/permissions';
import {
  deleteWorkspace,
  getWorkspace,
  getWorkspaceObjects,
  WORKSPACE_ERRORS,
} from '../../services/workspacesService';

/** Why a Workspace cannot be deleted now, or '' when it can. */
export const deleteBlockedReason = (workspace, bindingCount) => {
  if (workspace?.attributes?.is_default) return 'The Default workspace cannot be deleted.';
  if (bindingCount > 0) return 'Roles are still assigned in this Workspace; remove them on the Access tab first.';
  return '';
};

/** The message for a refused delete, by the API's error code. */
export const deleteErrorMessage = (err) => {
  switch (err?.code) {
    case WORKSPACE_ERRORS.DEFAULT_LOCKED:
      return 'The Default workspace cannot be deleted.';
    case WORKSPACE_ERRORS.IN_USE:
      return 'Roles are still assigned in this Workspace; remove them on the Access tab first.';
    default:
      return err?.message || 'Failed to delete the Workspace';
  }
};

/**
 * One Workspace: its Access (who holds a role in it) and Objects (what it
 * owns). Deleting moves its objects to the Default workspace; it is refused
 * for the Default and while roles are assigned in it.
 */
const WorkspaceDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const { can } = usePermissions();
  const [workspace, setWorkspace] = useState(null);
  const [counts, setCounts] = useState(null);
  const [bindingCount, setBindingCount] = useState(0);
  const [tab, setTab] = useState(0);
  const [error, setError] = useState('');
  const [confirmOpen, setConfirmOpen] = useState(false);

  useEffect(() => {
    getWorkspace(id)
      .then(setWorkspace)
      .catch((err) => setError(err.message || 'Failed to load the Workspace'));
    getWorkspaceObjects(id)
      .then((o) => setCounts(o?.counts || {}))
      .catch(() => setCounts({}));
  }, [id]);

  const onBindingsChange = useCallback((n) => setBindingCount(n), []);

  const handleDelete = async () => {
    setConfirmOpen(false);
    try {
      await deleteWorkspace(id);
      navigate('/admin/workspaces');
    } catch (err) {
      setError(deleteErrorMessage(err));
    }
  };

  const blocked = deleteBlockedReason(workspace, bindingCount);
  const name = workspace?.attributes?.name || '';

  return (
    <>
      <TitleBox top="var(--studio-header-height)">
        <Stack direction="row" spacing={1} alignItems="center">
          <Typography variant="headingXLarge">{name}</Typography>
          {workspace?.attributes?.is_default && <Chip size="small" label="Default" />}
        </Stack>
        <Stack direction="row" spacing={1}>
          {can(P.WORKSPACES_WRITE) && (
            <PrimaryOutlineButton onClick={() => navigate(`/admin/workspaces/edit/${id}`)}>Edit</PrimaryOutlineButton>
          )}
          {can(P.WORKSPACES_DELETE) && (
            <Tooltip title={blocked}>
              <span>
                <DangerOutlineButton disabled={Boolean(blocked)} onClick={() => setConfirmOpen(true)} data-testid="delete-workspace">
                  Delete
                </DangerOutlineButton>
              </span>
            </Tooltip>
          )}
        </Stack>
      </TitleBox>
      <ContentBox>
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>
            {error}
          </Alert>
        )}
        {workspace?.attributes?.description && (
          <Typography variant="bodyMediumDefault" sx={{ mb: 2, display: 'block' }}>
            {workspace.attributes.description}
          </Typography>
        )}
        <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 2 }}>
          <Tab label="Access" />
          <Tab label="Objects" />
        </Tabs>
        <Box hidden={tab !== 0}>
          <WorkspaceAccessTab workspaceId={id} onBindingsChange={onBindingsChange} />
        </Box>
        <Box hidden={tab !== 1}>{counts && <WorkspaceObjectsTab workspaceId={id} counts={counts} />}</Box>
      </ContentBox>
      <ConfirmationDialog
        open={confirmOpen}
        title="Delete Workspace"
        message={`Deleting "${name}" moves every object it owns to the Default workspace. This cannot be undone.`}
        buttonLabel="Delete Workspace"
        onConfirm={handleDelete}
        onCancel={() => setConfirmOpen(false)}
        iconName="hexagon-exclamation"
        iconColor="background.buttonCritical"
        titleColor="text.criticalDefault"
        backgroundColor="background.surfaceCriticalDefault"
      />
    </>
  );
};

export default WorkspaceDetail;
