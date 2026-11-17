import React, { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import { Alert, Box, Stack, TextField, Typography } from '@mui/material';
import { TitleBox, ContentBox, PrimaryButton, SecondaryOutlineButton } from '../../styles/sharedStyles';
import { createWorkspace, getWorkspace, updateWorkspace } from '../../services/workspacesService';

/**
 * Create or edit a Workspace: name and description. A name another
 * Workspace has (409, case-insensitive on the server) is shown on the field.
 */
const WorkspaceForm = () => {
  const { id } = useParams();
  const editing = Boolean(id);
  const navigate = useNavigate();
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [nameError, setNameError] = useState('');
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!editing) return;
    getWorkspace(id)
      .then((w) => {
        setName(w.attributes?.name || '');
        setDescription(w.attributes?.description || '');
      })
      .catch((err) => setError(err.message || 'Failed to load the Workspace'));
  }, [editing, id]);

  const submit = async (e) => {
    e.preventDefault();
    setNameError('');
    setError('');
    if (!name.trim()) {
      setNameError('A name is required');
      return;
    }
    setSaving(true);
    try {
      const saved = editing
        ? await updateWorkspace(id, { name: name.trim(), description })
        : await createWorkspace({ name: name.trim(), description });
      navigate(`/admin/workspaces/${saved?.id || id}`);
    } catch (err) {
      if (err.status === 409 && !err.code) {
        setNameError('A Workspace with this name already exists');
      } else if (err.status === 400) {
        setNameError(err.message);
      } else {
        setError(err.message || 'Failed to save the Workspace');
      }
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <TitleBox top="var(--studio-header-height)">
        <Typography variant="headingXLarge">{editing ? 'Edit Workspace' : 'Add Workspace'}</Typography>
      </TitleBox>
      <ContentBox>
        <Box component="form" onSubmit={submit} noValidate sx={{ maxWidth: 560 }}>
          <Stack spacing={2}>
            {error && <Alert severity="error">{error}</Alert>}
            <TextField
              label="Name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              error={Boolean(nameError)}
              helperText={nameError || ' '}
              required
              inputProps={{ 'data-testid': 'workspace-name' }}
            />
            <TextField
              label="Description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              multiline
              minRows={2}
              inputProps={{ 'data-testid': 'workspace-description' }}
            />
            <Stack direction="row" spacing={1}>
              <PrimaryButton type="submit" variant="contained" disabled={saving}>
                {editing ? 'Save' : 'Add Workspace'}
              </PrimaryButton>
              <SecondaryOutlineButton onClick={() => navigate(editing ? `/admin/workspaces/${id}` : '/admin/workspaces')}>
                Cancel
              </SecondaryOutlineButton>
            </Stack>
          </Stack>
        </Box>
      </ContentBox>
    </>
  );
};

export default WorkspaceForm;
