import React, { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft';
import { Alert, Box, CircularProgress, Snackbar, TextField, Typography } from '@mui/material';
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  TitleContentBox,
  PrimaryButton,
  DangerOutlineButton,
} from '../../styles/sharedStyles';
import Section from '../common/Section';
import ConfirmationDialog from '../common/ConfirmationDialog';
import EnterpriseFeatureBadge from '../common/EnterpriseFeatureBadge';
import PermissionMatrix from './PermissionMatrix';
import OrphanedPermissions from './OrphanedPermissions';
import { getRole, createRole, updateRole, deleteRole } from '../../services/rbacService';
import { isEnterpriseFeature } from '../../utils/apiErrors';

const RoleForm = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [permissions, setPermissions] = useState(new Set());
  const [loading, setLoading] = useState(Boolean(id));
  const [saving, setSaving] = useState(false);
  const [enterpriseAvailable, setEnterpriseAvailable] = useState(true);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [snackbar, setSnackbar] = useState({ open: false, message: '', severity: 'success' });

  const notify = (message, severity = 'success') => setSnackbar({ open: true, message, severity });

  const load = useCallback(async () => {
    if (!id) return;
    try {
      setLoading(true);
      const role = await getRole(id);
      if (role.attributes.is_system) {
        // System roles are read-only; send the user to the detail page.
        navigate(`/admin/roles/${id}`, { replace: true, state: { notice: 'System roles are read-only. Clone the role to customise it.' } });
        return;
      }
      setName(role.attributes.name);
      setDescription(role.attributes.description || '');
      setPermissions(new Set(role.attributes.permissions || []));
    } catch (err) {
      if (isEnterpriseFeature(err)) setEnterpriseAvailable(false);
      else notify(err.message || 'Failed to load role', 'error');
    } finally {
      setLoading(false);
    }
  }, [id, navigate]);

  useEffect(() => {
    load();
  }, [load]);

  const handleSubmit = async (event) => {
    event.preventDefault();
    setSaving(true);
    try {
      const payload = { name: name.trim(), description: description.trim(), permissions: [...permissions] };
      if (id) {
        await updateRole(id, payload);
        notify('Role updated');
        navigate(`/admin/roles/${id}`);
      } else {
        const created = await createRole(payload);
        notify('Role created');
        navigate(`/admin/roles/${created.id}`);
      }
    } catch (err) {
      notify(err.message || 'Failed to save role', 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async () => {
    try {
      await deleteRole(id);
      navigate('/admin/roles');
    } catch (err) {
      notify(err.message || 'Failed to delete role', 'error');
      setDeleteOpen(false);
    }
  };

  if (!enterpriseAvailable) {
    return <EnterpriseFeatureBadge feature="Role-based access control" />;
  }
  if (loading) return <CircularProgress />;

  return (
    <>
      <TitleBox>
        <TitleContentBox>
          <SecondaryLinkButton component={Link} to="/admin/roles" color="inherit" sx={{ mb: 1, px: 0 }} startIcon={<ChevronLeftIcon sx={{ mr: -1 }} />}>
            back to roles
          </SecondaryLinkButton>
          <Typography variant="headingXLarge">{id ? 'Edit role' : 'Create role'}</Typography>
        </TitleContentBox>
      </TitleBox>

      <ContentBox sx={{ maxWidth: { xs: '100%', lg: '85%' } }}>
        <form onSubmit={handleSubmit}>
          <Section title="Basic information">
            <Box sx={{ px: 2, pb: 2, display: 'flex', flexDirection: 'column', gap: 2 }}>
              <TextField
                label="Name"
                required
                size="small"
                value={name}
                onChange={(e) => setName(e.target.value)}
                inputProps={{ 'data-testid': 'role-name' }}
              />
              <TextField
                label="Description"
                size="small"
                multiline
                minRows={2}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                inputProps={{ 'data-testid': 'role-description' }}
              />
            </Box>
          </Section>

          <Section title="Permissions">
            <Box sx={{ px: 2, pb: 2 }}>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Tick what this role may do. Write, delete and execute each include read. Rows marked with a shield expose sensitive data; rows marked with a key can grant access to other people.
              </Typography>
              <PermissionMatrix value={permissions} onChange={setPermissions} />
              <OrphanedPermissions
                permissions={permissions}
                onRemove={(perm) => setPermissions((prev) => { const next = new Set(prev); next.delete(perm); return next; })}
              />
            </Box>
          </Section>

          <Box sx={{ display: 'flex', justifyContent: 'flex-start', mt: 3, gap: 2 }}>
            <PrimaryButton type="submit" disabled={saving || !name.trim()}>
              {id ? 'Update role' : 'Create role'}
            </PrimaryButton>
            {id && <DangerOutlineButton onClick={() => setDeleteOpen(true)}>Delete role</DangerOutlineButton>}
          </Box>
        </form>
      </ContentBox>

      <ConfirmationDialog
        open={deleteOpen}
        title="Delete role"
        message="Deleting this role removes it from every user and team it is assigned to."
        buttonLabel="Delete role"
        onConfirm={handleDelete}
        onCancel={() => setDeleteOpen(false)}
        iconName="hexagon-exclamation"
        iconColor="background.buttonCritical"
        titleColor="text.criticalDefault"
        backgroundColor="background.surfaceCriticalDefault"
        borderColor="border.criticalDefaultSubdue"
        primaryButtonComponent="danger"
      />

      <Snackbar open={snackbar.open} autoHideDuration={6000} onClose={() => setSnackbar((s) => ({ ...s, open: false }))} anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}>
        <Alert severity={snackbar.severity} onClose={() => setSnackbar((s) => ({ ...s, open: false }))}>{snackbar.message}</Alert>
      </Snackbar>
    </>
  );
};

export default RoleForm;
