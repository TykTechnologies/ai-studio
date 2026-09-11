import React, { useCallback, useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import ChevronLeftIcon from '@mui/icons-material/ChevronLeft';
import { Alert, Box, CircularProgress, Snackbar, Typography } from '@mui/material';
import {
  SecondaryLinkButton,
  TitleBox,
  ContentBox,
  TitleContentBox,
  PrimaryButton,
  SecondaryOutlineButton,
  FieldLabel,
  FieldValue,
} from '../../styles/sharedStyles';
import Section from '../common/Section';
import EnterpriseFeatureBadge from '../common/EnterpriseFeatureBadge';
import PermissionMatrix from './PermissionMatrix';
import CloneRoleDialog from './CloneRoleDialog';
import RoleBadge, { SystemBadge } from './RoleBadge';
import Can from '../rbac/Can';
import { P } from '../../rbac/permissions';
import { getRole, listBindings, cloneRole } from '../../services/rbacService';
import { isEnterpriseFeature } from '../../utils/apiErrors';

const RoleDetail = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [role, setRole] = useState(null);
  const [bindings, setBindings] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [enterpriseAvailable, setEnterpriseAvailable] = useState(true);
  const [cloneOpen, setCloneOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [snackbar, setSnackbar] = useState({ open: false, message: '', severity: 'success' });

  const load = useCallback(async () => {
    try {
      setLoading(true);
      const [r, b] = await Promise.all([getRole(id), listBindings({ roleId: id })]);
      setRole(r);
      setBindings(b);
      setError('');
    } catch (err) {
      if (isEnterpriseFeature(err)) setEnterpriseAvailable(false);
      else setError(err.message || 'Failed to load role');
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    load();
  }, [load]);

  const handleClone = async (name) => {
    setBusy(true);
    try {
      const created = await cloneRole(id, name);
      setCloneOpen(false);
      navigate(`/admin/roles/edit/${created.id}`);
    } catch (err) {
      setSnackbar({ open: true, message: err.message || 'Failed to clone role', severity: 'error' });
    } finally {
      setBusy(false);
    }
  };

  if (!enterpriseAvailable) {
    return <EnterpriseFeatureBadge feature="Role-based access control" />;
  }
  if (loading) return <CircularProgress />;
  if (error || !role) return <Alert severity="error">{error || 'Role not found'}</Alert>;

  const a = role.attributes;
  const users = bindings.filter((b) => b.attributes.subject_type === 'user');
  const teams = bindings.filter((b) => b.attributes.subject_type === 'group');
  const permissions = new Set(a.permissions || []);

  return (
    <>
      <TitleBox>
        <TitleContentBox>
          <SecondaryLinkButton component={Link} to="/admin/roles" color="inherit" sx={{ mb: 1, px: 0 }} startIcon={<ChevronLeftIcon sx={{ mr: -1 }} />}>
            back to roles
          </SecondaryLinkButton>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
            <Typography variant="headingXLarge">{a.name}</Typography>
            <SystemBadge isSystem={a.is_system} />
          </Box>
        </TitleContentBox>
        <Box sx={{ display: 'flex', gap: 1 }}>
          <Can permission={P.ROLES_WRITE}>
            <SecondaryOutlineButton onClick={() => setCloneOpen(true)}>Clone role</SecondaryOutlineButton>
            {!a.is_system && (
              <PrimaryButton onClick={() => navigate(`/admin/roles/edit/${role.id}`)}>Edit role</PrimaryButton>
            )}
          </Can>
        </Box>
      </TitleBox>

      <ContentBox>
        <Section title="Details">
          <Box sx={{ px: 2, pb: 2 }}>
            <FieldLabel>Description</FieldLabel>
            <FieldValue>{a.description || '—'}</FieldValue>
            <FieldLabel sx={{ mt: 2 }}>Assigned to</FieldLabel>
            <FieldValue>
              {users.length} {users.length === 1 ? 'user' : 'users'}, {teams.length} {teams.length === 1 ? 'team' : 'teams'}
            </FieldValue>
            {a.is_system && (
              <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
                System roles are read-only and kept up to date automatically. Clone this role to customise it.
              </Typography>
            )}
          </Box>
        </Section>

        <Section title="Permissions">
          <Box sx={{ px: 2, pb: 2 }}>
            {permissions.has('*') ? (
              <Typography>Full administrator access: every permission, including managing roles and users.</Typography>
            ) : (
              <PermissionMatrix value={permissions} readOnly showSearch={false} />
            )}
          </Box>
        </Section>

        {bindings.length > 0 && (
          <Section title="Assignments">
            <Box sx={{ px: 2, pb: 2 }}>
              {bindings.map((b) => (
                <Typography key={b.id} variant="body2">
                  {b.attributes.subject_type === 'user' ? 'User' : 'Team'} #{b.attributes.subject_id}
                </Typography>
              ))}
            </Box>
          </Section>
        )}
        <Box sx={{ mt: 2 }}>
          <RoleBadge role={role} />
        </Box>
      </ContentBox>

      <CloneRoleDialog open={cloneOpen} role={role} busy={busy} onConfirm={handleClone} onCancel={() => setCloneOpen(false)} />
      <Snackbar open={snackbar.open} autoHideDuration={6000} onClose={() => setSnackbar((s) => ({ ...s, open: false }))} anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}>
        <Alert severity={snackbar.severity} onClose={() => setSnackbar((s) => ({ ...s, open: false }))}>{snackbar.message}</Alert>
      </Snackbar>
    </>
  );
};

export default RoleDetail;
