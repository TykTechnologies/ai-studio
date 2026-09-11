import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import AddIcon from '@mui/icons-material/Add';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import {
  Alert,
  Box,
  CircularProgress,
  IconButton,
  Menu,
  MenuItem,
  Snackbar,
  Table,
  TableBody,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import {
  StyledPaper,
  TitleBox,
  ContentBox,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
  PrimaryButton,
} from '../../styles/sharedStyles';
import EmptyStateWidget from '../../components/common/EmptyStateWidget';
import EnterpriseFeatureBadge from '../../components/common/EnterpriseFeatureBadge';
import ConfirmationDialog from '../../components/common/ConfirmationDialog';
import CloneRoleDialog from '../../components/roles/CloneRoleDialog';
import { SystemBadge } from '../../components/roles/RoleBadge';
import Can from '../../components/rbac/Can';
import { usePermissions } from '../../context/PermissionsContext';
import { P } from '../../rbac/permissions';
import { listRoles, deleteRole, cloneRole, sortRoles } from '../../services/rbacService';
import { isEnterpriseFeature, isPermissionDenied } from '../../utils/apiErrors';

const Roles = () => {
  const navigate = useNavigate();
  const { can } = usePermissions();
  const [roles, setRoles] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [enterpriseAvailable, setEnterpriseAvailable] = useState(true);
  const [anchorEl, setAnchorEl] = useState(null);
  const [selected, setSelected] = useState(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [cloneOpen, setCloneOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [snackbar, setSnackbar] = useState({ open: false, message: '', severity: 'success' });

  const canManage = can(P.ROLES_WRITE);

  const fetchRoles = useCallback(async () => {
    try {
      setLoading(true);
      const list = await listRoles();
      setRoles(sortRoles(list));
      setError('');
    } catch (err) {
      if (isEnterpriseFeature(err)) {
        setEnterpriseAvailable(false);
      } else if (isPermissionDenied(err)) {
        setError('Your role does not include access to roles');
      } else {
        setError('Failed to load roles');
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchRoles();
  }, [fetchRoles]);

  const openMenu = (event, role) => {
    event.stopPropagation();
    setAnchorEl(event.currentTarget);
    setSelected(role);
  };
  const closeMenu = () => setAnchorEl(null);

  const notify = (message, severity = 'success') => setSnackbar({ open: true, message, severity });

  const handleDelete = async () => {
    if (!selected) return;
    setBusy(true);
    try {
      await deleteRole(selected.id);
      notify(`Role "${selected.attributes.name}" deleted`);
      setDeleteOpen(false);
      fetchRoles();
    } catch (err) {
      notify(err.message || 'Failed to delete role', 'error');
    } finally {
      setBusy(false);
    }
  };

  const handleClone = async (name) => {
    if (!selected) return;
    setBusy(true);
    try {
      const created = await cloneRole(selected.id, name);
      setCloneOpen(false);
      notify(`Role "${created.attributes.name}" created`);
      navigate(`/admin/roles/edit/${created.id}`);
    } catch (err) {
      notify(err.message || 'Failed to clone role', 'error');
    } finally {
      setBusy(false);
    }
  };

  if (!enterpriseAvailable) {
    return (
      <>
        <TitleBox top="64px">
          <Typography variant="headingXLarge">Roles</Typography>
        </TitleBox>
        <ContentBox>
          <EnterpriseFeatureBadge
            feature="Role-based access control"
            description="Define fine-grained roles and assign them to users and teams. Roles apply across the administration UI and the API."
          />
        </ContentBox>
      </>
    );
  }

  if (loading && roles.length === 0) {
    return <CircularProgress />;
  }

  return (
    <>
      <TitleBox top="64px">
        <Typography variant="headingXLarge">Roles</Typography>
        <Can permission={P.ROLES_WRITE}>
          <PrimaryButton variant="contained" startIcon={<AddIcon />} onClick={() => navigate('/admin/roles/new')}>
            Add role
          </PrimaryButton>
        </Can>
      </TitleBox>
      <ContentBox>
        {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}
        {roles.length === 0 && !error ? (
          <EmptyStateWidget
            title="No roles found"
            description="System roles are created automatically. Add a custom role to tailor access."
            buttonText={canManage ? 'Add role' : undefined}
            buttonIcon={canManage ? <AddIcon /> : undefined}
            onButtonClick={canManage ? () => navigate('/admin/roles/new') : undefined}
          />
        ) : (
          <StyledPaper>
            <Table>
              <TableHead>
                <TableRow>
                  <StyledTableHeaderCell>Name</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Type</StyledTableHeaderCell>
                  <StyledTableHeaderCell>Description</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="right">Permissions</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="right">Assigned to</StyledTableHeaderCell>
                  <StyledTableHeaderCell align="right">Actions</StyledTableHeaderCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {roles.map((role) => {
                  const a = role.attributes;
                  const permCount = a.permissions?.includes('*') ? 'All' : (a.permissions?.length ?? 0);
                  return (
                    <StyledTableRow
                      key={role.id}
                      onClick={() => navigate(`/admin/roles/${role.id}`)}
                      sx={{ cursor: 'pointer' }}
                      data-testid={`role-row-${a.slug}`}
                    >
                      <StyledTableCell>{a.name}</StyledTableCell>
                      <StyledTableCell><SystemBadge isSystem={a.is_system} /></StyledTableCell>
                      <StyledTableCell>{a.description}</StyledTableCell>
                      <StyledTableCell align="right">
                        <Tooltip title={a.permissions?.includes('*') ? 'Full administrator access' : (a.permissions || []).slice(0, 12).join(', ')}>
                          <span>{permCount}</span>
                        </Tooltip>
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        {a.users_count} {a.users_count === 1 ? 'user' : 'users'}, {a.groups_count} {a.groups_count === 1 ? 'team' : 'teams'}
                      </StyledTableCell>
                      <StyledTableCell align="right">
                        {canManage && (
                          <IconButton onClick={(e) => openMenu(e, role)} aria-label={`Actions for ${a.name}`}>
                            <MoreVertIcon />
                          </IconButton>
                        )}
                      </StyledTableCell>
                    </StyledTableRow>
                  );
                })}
              </TableBody>
            </Table>
          </StyledPaper>
        )}
      </ContentBox>

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={closeMenu}>
        <MenuItem onClick={() => { closeMenu(); navigate(`/admin/roles/${selected?.id}`); }}>View role</MenuItem>
        <MenuItem onClick={() => { closeMenu(); setCloneOpen(true); }}>Clone role</MenuItem>
        {selected?.attributes?.is_system ? (
          <Tooltip title="System roles cannot be edited. Clone it to customise." placement="left">
            <Box>
              <MenuItem disabled>Edit role</MenuItem>
              <MenuItem disabled>Delete role</MenuItem>
            </Box>
          </Tooltip>
        ) : (
          <Box>
            <MenuItem onClick={() => { closeMenu(); navigate(`/admin/roles/edit/${selected?.id}`); }}>Edit role</MenuItem>
            <MenuItem onClick={() => { closeMenu(); setDeleteOpen(true); }}>Delete role</MenuItem>
          </Box>
        )}
      </Menu>

      <CloneRoleDialog open={cloneOpen} role={selected} busy={busy} onConfirm={handleClone} onCancel={() => setCloneOpen(false)} />

      <ConfirmationDialog
        open={deleteOpen}
        title="Delete role"
        message={`Deleting "${selected?.attributes?.name}" removes it from every user and team it is assigned to.`}
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

      <Snackbar
        open={snackbar.open}
        autoHideDuration={6000}
        onClose={() => setSnackbar((s) => ({ ...s, open: false }))}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert onClose={() => setSnackbar((s) => ({ ...s, open: false }))} severity={snackbar.severity} sx={{ width: '100%' }}>
          {snackbar.message}
        </Alert>
      </Snackbar>
    </>
  );
};

export default Roles;
