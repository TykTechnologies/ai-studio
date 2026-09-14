import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import AddIcon from '@mui/icons-material/Add';
import { Alert, Tooltip, Typography } from '@mui/material';
import {
  TitleBox,
  ContentBox,
  PrimaryButton,
} from '../../styles/sharedStyles';
import DataTable from '../../components/common/DataTable';
import EmptyStateWidget from '../../components/common/EmptyStateWidget';
import EnterpriseFeatureBadge from '../../components/common/EnterpriseFeatureBadge';
import ConfirmationDialog from '../../components/common/ConfirmationDialog';
import BulkDeleteConfirmationDialog from '../../components/common/BulkDeleteConfirmationDialog';
import BulkResultAlert from '../../components/common/BulkResultAlert';
import FeedbackSnackbar, { useFeedbackSnackbar } from '../../components/common/FeedbackSnackbar';
import CloneRoleDialog from '../../components/roles/CloneRoleDialog';
import { SystemBadge } from '../../components/roles/RoleBadge';
import Can from '../../components/rbac/Can';
import { usePermissions } from '../../context/PermissionsContext';
import { P } from '../../rbac/permissions';
import useBulkActions, { standardBulkActions } from '../../hooks/useBulkActions';
import { listRoles, deleteRole, cloneRole, sortRoles } from '../../services/rbacService';
import { isEnterpriseFeature, isPermissionDenied } from '../../utils/apiErrors';

const SYSTEM_ROLE_REASON = 'System roles cannot be edited. Clone it to customise.';

// /rbac/roles takes no search or sort parameters, so both happen here.
const matchesSearch = (role, term) => {
  if (!term) return true;
  const needle = term.toLowerCase();
  const a = role.attributes || {};
  return [a.name, a.slug, a.description].some((value) => String(value || '').toLowerCase().includes(needle));
};

const compareRoles = (sortConfig) => (left, right) => {
  const field = sortConfig?.field;
  const direction = sortConfig?.direction === 'desc' ? -1 : 1;
  const l = left.attributes?.[field];
  const r = right.attributes?.[field];
  if (typeof l === 'number' && typeof r === 'number') return (l - r) * direction;
  return String(l ?? '').localeCompare(String(r ?? ''), undefined, { sensitivity: 'base' }) * direction;
};

const Roles = () => {
  const navigate = useNavigate();
  const { can } = usePermissions();
  const [roles, setRoles] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [enterpriseAvailable, setEnterpriseAvailable] = useState(true);
  const [selected, setSelected] = useState(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [cloneOpen, setCloneOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [searchTerm, setSearchTerm] = useState('');
  const [sortConfig, setSortConfig] = useState(null);
  const { notify, snackbarProps } = useFeedbackSnackbar();

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

  const visibleRoles = useMemo(() => {
    const filtered = roles.filter((role) => matchesSearch(role, searchTerm));
    return sortConfig?.field ? [...filtered].sort(compareRoles(sortConfig)) : filtered;
  }, [roles, searchTerm, sortConfig]);

  const bulk = useBulkActions({
    items: visibleRoles,
    resource: 'rbac/roles',
    singular: 'role',
    plural: 'roles',
    notify,
    refresh: fetchRoles,
    viaBulkEndpoint: false,
    deleteOne: deleteRole,
  });

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

  const columns = useMemo(() => [
    { field: 'name', headerName: 'Name', sortable: true, renderCell: (role) => role.attributes.name },
    { field: 'is_system', headerName: 'Type', renderCell: (role) => <SystemBadge isSystem={role.attributes.is_system} /> },
    { field: 'description', headerName: 'Description', sortable: true, renderCell: (role) => role.attributes.description },
    {
      field: 'permissions',
      headerName: 'Permissions',
      align: 'right',
      renderCell: (role) => {
        const a = role.attributes;
        const permCount = a.permissions?.includes('*') ? 'All' : (a.permissions?.length ?? 0);
        return (
          <Tooltip title={a.permissions?.includes('*') ? 'Full administrator access' : (a.permissions || []).slice(0, 12).join(', ')}>
            <span>{permCount}</span>
          </Tooltip>
        );
      },
    },
    {
      field: 'users_count',
      headerName: 'Assigned to',
      align: 'right',
      sortable: true,
      renderCell: (role) => {
        const a = role.attributes;
        return `${a.users_count} ${a.users_count === 1 ? 'user' : 'users'}, ${a.groups_count} ${a.groups_count === 1 ? 'team' : 'teams'}`;
      },
    },
  ], []);

  const rowActions = useMemo(() => [
    { key: 'view', label: 'View role', onClick: (role) => navigate(`/admin/roles/${role.id}`) },
    { key: 'clone', label: 'Clone role', onClick: (role) => { setSelected(role); setCloneOpen(true); } },
    {
      key: 'edit',
      label: 'Edit role',
      disabled: (role) => Boolean(role?.attributes?.is_system),
      disabledReason: SYSTEM_ROLE_REASON,
      onClick: (role) => navigate(`/admin/roles/edit/${role.id}`),
    },
    {
      key: 'delete',
      label: 'Delete role',
      disabled: (role) => Boolean(role?.attributes?.is_system),
      disabledReason: SYSTEM_ROLE_REASON,
      onClick: (role) => { setSelected(role); setDeleteOpen(true); },
    },
  ], [navigate]);

  const bulkActions = useMemo(
    () =>
      standardBulkActions({ run: bulk.run, requestDelete: bulk.requestDelete }).map((action) =>
        action.key === 'delete'
          ? {
              ...action,
              disabled: (items) => items.some((role) => role.attributes?.is_system),
              disabledReason: 'System roles cannot be deleted; clear them from the selection.',
            }
          : action,
      ),
    [bulk.run, bulk.requestDelete],
  );

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
        <BulkResultAlert action={bulk.failures?.action} failures={bulk.failures?.failures} onClose={bulk.clearFailures} />
        <DataTable
          ariaLabel="Roles"
          enableSearch
          searchTerm={searchTerm}
          onSearch={setSearchTerm}
          searchPlaceholder="Search roles by name..."
          sortConfig={sortConfig}
          onSortChange={setSortConfig}
          columns={columns}
          data={visibleRoles}
          loading={loading}
          onRowClick={(role) => navigate(`/admin/roles/${role.id}`)}
          rowProps={(role) => ({ 'data-testid': `role-row-${role.attributes.slug}` })}
          actions={canManage ? rowActions : undefined}
          {...(canManage ? bulk.selectionProps : {})}
          bulkActions={canManage ? bulkActions : undefined}
          emptyState={
            !searchTerm && !error ? (
              <EmptyStateWidget
                title="No roles found"
                description="System roles are created automatically. Add a custom role to tailor access."
                buttonText={canManage ? 'Add role' : undefined}
                buttonIcon={canManage ? <AddIcon /> : undefined}
                onButtonClick={canManage ? () => navigate('/admin/roles/new') : undefined}
              />
            ) : undefined
          }
        />
      </ContentBox>

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

      <BulkDeleteConfirmationDialog
        open={bulk.deleteDialogOpen}
        resourcePath={null}
        objectLabel="role"
        objectLabelPlural="roles"
        items={bulk.deleteDialogItems}
        onConfirm={bulk.confirmDelete}
        onCancel={bulk.cancelDelete}
      />

      <FeedbackSnackbar {...snackbarProps} />
    </>
  );
};

export default Roles;
