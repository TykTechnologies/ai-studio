import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert,
  Box,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  IconButton,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import { Edit as EditIcon, Delete as DeleteIcon } from '@mui/icons-material';
import { PrimaryButton, SecondaryOutlineButton } from '../../styles/sharedStyles';
import RoleSelect from '../roles/RoleSelect';
import { listBindings, listRoles, sortRoles, syncScopedBindings } from '../../services/rbacService';
import apiClient from '../../utils/apiClient';
import { listAll } from '../../utils/listAll';
import { usePermissions } from '../../context/PermissionsContext';
import { P } from '../../rbac/permissions';

const OWNER_SLUG = 'owner';
const subjectKey = (type, id) => `${type}:${id}`;

/**
 * Who holds a role inside one Workspace: users (service accounts included)
 * and teams with Workspace-scoped bindings. Adding, changing and removing
 * them goes through syncScopedBindings, which never touches global
 * bindings. Managing them needs roles:write globally (D10). The Owner role
 * is global only and not offered.
 */
const WorkspaceAccessTab = ({ workspaceId, onBindingsChange }) => {
  const { can } = usePermissions();
  const canManage = can(P.ROLES_WRITE);
  const [bindings, setBindings] = useState([]);
  const [roles, setRoles] = useState([]);
  const [users, setUsers] = useState([]);
  const [teams, setTeams] = useState([]);
  const [error, setError] = useState('');
  const [editing, setEditing] = useState(null); // { type, id, roleIds, isNew }
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    try {
      const list = await listBindings({ scopeType: 'workspace', scopeId: String(workspaceId) });
      const scoped = list.filter(
        (b) => b.attributes.scope_type === 'workspace' && String(b.attributes.scope_id) === String(workspaceId)
      );
      setBindings(scoped);
      onBindingsChange?.(scoped.length);
      setError('');
    } catch (err) {
      setError(err.message || 'Failed to load the role assignments');
    }
  }, [workspaceId, onBindingsChange]);

  useEffect(() => {
    load();
  }, [load]);

  useEffect(() => {
    listRoles()
      .then((list) => setRoles(sortRoles(list).filter((r) => r.attributes.slug !== OWNER_SLUG)))
      .catch(() => setRoles([]));
    listAll(apiClient, '/users')
      .then((res) => setUsers(res.data?.data || []))
      .catch(() => setUsers([]));
    listAll(apiClient, '/groups')
      .then((res) => setTeams(res.data?.data || []))
      .catch(() => setTeams([]));
  }, []);

  const names = useMemo(() => {
    const m = new Map();
    users.forEach((u) => {
      const sa = u.attributes?.account_type === 'service';
      m.set(subjectKey('user', u.id), `${u.attributes?.name || u.attributes?.email}${sa ? ' (service account)' : ''}`);
    });
    teams.forEach((t) => m.set(subjectKey('group', t.id), t.attributes?.name));
    return m;
  }, [users, teams]);

  const rows = useMemo(() => {
    const bySubject = new Map();
    bindings.forEach((b) => {
      const key = subjectKey(b.attributes.subject_type, b.attributes.subject_id);
      if (!bySubject.has(key)) {
        bySubject.set(key, { type: b.attributes.subject_type, id: String(b.attributes.subject_id), roles: [] });
      }
      bySubject.get(key).roles.push({ id: Number(b.attributes.role_id), name: b.attributes.role?.name });
    });
    return [...bySubject.values()];
  }, [bindings]);

  const save = async () => {
    setSaving(true);
    try {
      const res = await syncScopedBindings(workspaceId, editing.type, editing.id, editing.roleIds);
      if (res.failed.length > 0) {
        setError(res.failed.map((f) => f.error?.message || String(f.error)).join('; '));
      }
      setEditing(null);
      await load();
    } catch (err) {
      setError(err.message || 'Failed to save the role assignment');
    } finally {
      setSaving(false);
    }
  };

  const remove = async (row) => {
    try {
      await syncScopedBindings(workspaceId, row.type, row.id, []);
      await load();
    } catch (err) {
      setError(err.message || 'Failed to remove the role assignment');
    }
  };

  const subjectOptions = editing?.type === 'group' ? teams : users;
  const taken = new Set(rows.map((r) => subjectKey(r.type, r.id)));

  return (
    <Box>
      <Alert severity="info" sx={{ mb: 2 }}>
        A role assigned here applies only to this Workspace's objects. Permissions in the role on global resources (users,
        roles, plugins, settings and the like) do not apply inside a Workspace; give those with a global role.
      </Alert>
      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>
          {error}
        </Alert>
      )}
      {canManage && (
        <PrimaryButton
          variant="contained"
          sx={{ mb: 2 }}
          onClick={() => setEditing({ type: 'user', id: '', roleIds: [], isNew: true })}
        >
          Add user or team
        </PrimaryButton>
      )}
      <Table data-testid="workspace-access-table">
        <TableHead>
          <TableRow>
            <TableCell>User or team</TableCell>
            <TableCell>Roles in this Workspace</TableCell>
            {canManage && <TableCell align="right">Actions</TableCell>}
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.length === 0 && (
            <TableRow>
              <TableCell colSpan={3}>
                <Typography variant="bodyMediumDefault">No one holds a role in this Workspace yet.</Typography>
              </TableCell>
            </TableRow>
          )}
          {rows.map((row) => (
            <TableRow key={subjectKey(row.type, row.id)} data-testid={`access-row-${row.type}-${row.id}`}>
              <TableCell>
                {names.get(subjectKey(row.type, row.id)) || `${row.type === 'group' ? 'Team' : 'User'} ${row.id}`}
                <Chip size="small" sx={{ ml: 1 }} label={row.type === 'group' ? 'Team' : 'User'} />
              </TableCell>
              <TableCell>
                {row.roles.map((r) => (
                  <Chip key={r.id} size="small" sx={{ mr: 0.5 }} label={r.name || `Role ${r.id}`} />
                ))}
              </TableCell>
              {canManage && (
                <TableCell align="right">
                  <Tooltip title="Change roles">
                    <IconButton
                      aria-label="Change roles"
                      onClick={() => setEditing({ type: row.type, id: row.id, roleIds: row.roles.map((r) => r.id) })}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                  <Tooltip title="Remove from this Workspace">
                    <IconButton aria-label="Remove from this Workspace" onClick={() => remove(row)}>
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                </TableCell>
              )}
            </TableRow>
          ))}
        </TableBody>
      </Table>

      <Dialog open={Boolean(editing)} onClose={() => setEditing(null)} fullWidth maxWidth="sm">
        <DialogTitle>{editing?.isNew ? 'Add to this Workspace' : 'Change roles in this Workspace'}</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            {editing?.isNew && (
              <>
                <FormControl fullWidth size="small">
                  <InputLabel id="subject-type-label">Type</InputLabel>
                  <Select
                    labelId="subject-type-label"
                    label="Type"
                    value={editing.type}
                    onChange={(e) => setEditing({ ...editing, type: e.target.value, id: '' })}
                    inputProps={{ 'data-testid': 'subject-type' }}
                  >
                    <MenuItem value="user">User or service account</MenuItem>
                    <MenuItem value="group">Team</MenuItem>
                  </Select>
                </FormControl>
                <FormControl fullWidth size="small">
                  <InputLabel id="subject-label">{editing.type === 'group' ? 'Team' : 'User'}</InputLabel>
                  <Select
                    labelId="subject-label"
                    label={editing.type === 'group' ? 'Team' : 'User'}
                    value={editing.id}
                    onChange={(e) => setEditing({ ...editing, id: String(e.target.value) })}
                    inputProps={{ 'data-testid': 'subject-select' }}
                  >
                    {subjectOptions
                      .filter((s) => !taken.has(subjectKey(editing.type, s.id)))
                      .map((s) => (
                        <MenuItem key={s.id} value={String(s.id)}>
                          {names.get(subjectKey(editing.type, s.id))}
                        </MenuItem>
                      ))}
                  </Select>
                </FormControl>
              </>
            )}
            {editing && (
              <RoleSelect
                roles={roles}
                value={editing.roleIds}
                onChange={(ids) => setEditing({ ...editing, roleIds: ids })}
                label="Roles in this Workspace"
                id="workspace-role-select"
              />
            )}
          </Stack>
        </DialogContent>
        <DialogActions>
          <SecondaryOutlineButton onClick={() => setEditing(null)}>Cancel</SecondaryOutlineButton>
          <PrimaryButton
            variant="contained"
            onClick={save}
            disabled={saving || !editing?.id || (editing?.isNew && editing.roleIds.length === 0)}
          >
            Save
          </PrimaryButton>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default WorkspaceAccessTab;
