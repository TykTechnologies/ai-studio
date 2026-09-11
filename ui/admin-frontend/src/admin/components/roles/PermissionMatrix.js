import React, { useMemo, useState } from 'react';
import {
  Box,
  Checkbox,
  Table,
  TableBody,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
  TextField,
} from '@mui/material';
import Icon from '../../../components/common/Icon';
import {
  StyledPaper,
  StyledTableCell,
  StyledTableHeaderCell,
  StyledTableRow,
} from '../../styles/sharedStyles';
import usePermissionCatalogue from '../../hooks/usePermissionCatalogue';

const ACTION_LABELS = { read: 'Read', write: 'Write', delete: 'Delete', execute: 'Execute', publish: 'Publish' };
const ACTION_HINTS = {
  publish: 'Make live: activate or enable the object. Does not include write, so a role can review and release without editing, or draft without releasing.',
};

/**
 * One table per catalogue group; rows are resources, columns are actions.
 * Plugin-contributed resources render as one sub-table per plugin after the
 * group's built-in rows. `value` is a Set of "resource:action" strings.
 * Sensitive resources carry a shield, privileged ones a warning. Ticking
 * write/delete/execute/publish also ticks read, mirroring the backend's
 * implied-read rule.
 */
const PermissionMatrix = ({ value, onChange, readOnly = false, showSearch = true }) => {
  const { grouped, actions, loading } = usePermissionCatalogue();
  const [search, setSearch] = useState('');
  const held = value instanceof Set ? value : new Set(value || []);

  const visibleGroups = useMemo(() => {
    const q = search.trim().toLowerCase();
    const matches = (r) => !q || r.label.toLowerCase().includes(q) || r.key.includes(q);
    return grouped
      .map((g) => {
        const builtIn = (g.builtIn || g.resources).filter(matches);
        const plugins = (g.plugins || [])
          .map((p) => ({ ...p, resources: p.resources.filter((r) => matches(r) || p.label.toLowerCase().includes(q)) }))
          .filter((p) => p.resources.length > 0);
        return { group: g.group, builtIn, plugins, count: builtIn.length + plugins.reduce((n, p) => n + p.resources.length, 0) };
      })
      .filter((g) => g.count > 0);
  }, [grouped, search]);

  const toggle = (resource, action, checked) => {
    if (readOnly || !onChange) return;
    const next = new Set(held);
    const perm = `${resource.key}:${action}`;
    if (checked) {
      next.add(perm);
      if (action !== 'read') next.add(`${resource.key}:read`);
    } else {
      next.delete(perm);
      if (action === 'read') {
        // Dropping read drops everything on the resource.
        resource.actions.forEach((a) => next.delete(`${resource.key}:${a}`));
      }
    }
    onChange(next);
  };

  const toggleRow = (resource, checked) => {
    if (readOnly || !onChange) return;
    const next = new Set(held);
    resource.actions.forEach((a) => {
      const perm = `${resource.key}:${a}`;
      if (checked) next.add(perm);
      else next.delete(perm);
    });
    onChange(next);
  };

  const rowState = (resource) => {
    const count = resource.actions.filter((a) => held.has(`${resource.key}:${a}`)).length;
    return { all: count === resource.actions.length, some: count > 0 && count < resource.actions.length };
  };

  if (loading) {
    return <Typography color="text.secondary">Loading permissions…</Typography>;
  }

  const renderTable = (resources, tableKey) => (
    <Table size="small" key={tableKey} data-testid={`matrix-table-${tableKey}`}>
      <TableHead>
        <TableRow>
          <StyledTableHeaderCell>Resource</StyledTableHeaderCell>
          <StyledTableHeaderCell align="center">All</StyledTableHeaderCell>
          {actions.map((a) => (
            <StyledTableHeaderCell key={a} align="center">
              {ACTION_HINTS[a] ? (
                <Tooltip title={ACTION_HINTS[a]}>
                  <span>{ACTION_LABELS[a] || a}</span>
                </Tooltip>
              ) : (
                ACTION_LABELS[a] || a
              )}
            </StyledTableHeaderCell>
          ))}
        </TableRow>
      </TableHead>
      <TableBody>
        {resources.map((resource) => {
          const { all, some } = rowState(resource);
          return (
            <StyledTableRow key={resource.key} data-testid={`matrix-row-${resource.key}`}>
              <StyledTableCell>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <span>{resource.label}</span>
                  {resource.sensitive && (
                    <Tooltip title="Sensitive data: reading exposes logs, transcripts or secrets">
                      <span><Icon name="shield" style={{ fontSize: 14, color: '#b26a00' }} /></span>
                    </Tooltip>
                  )}
                  {resource.privileged && (
                    <Tooltip title="Privileged: write access here can grant access to others">
                      <span><Icon name="shield-keyhole" style={{ fontSize: 14, color: "#666" }} /></span>
                    </Tooltip>
                  )}
                  {resource.description && (
                    <Tooltip title={resource.description}>
                      <span><Icon name="circle-info" style={{ fontSize: 14, color: '#999' }} /></span>
                    </Tooltip>
                  )}
                </Box>
              </StyledTableCell>
              <StyledTableCell align="center">
                <Checkbox
                  size="small"
                  checked={all}
                  indeterminate={some}
                  disabled={readOnly}
                  onChange={(e) => toggleRow(resource, e.target.checked)}
                  inputProps={{ 'aria-label': `All ${resource.label} permissions` }}
                />
              </StyledTableCell>
              {actions.map((a) => {
                const offered = resource.actions.includes(a);
                const perm = `${resource.key}:${a}`;
                return (
                  <StyledTableCell key={a} align="center">
                    {offered ? (
                      <Checkbox
                        size="small"
                        checked={held.has(perm)}
                        disabled={readOnly}
                        onChange={(e) => toggle(resource, a, e.target.checked)}
                        inputProps={{ 'aria-label': perm }}
                      />
                    ) : (
                      <Typography component="span" color="text.disabled">—</Typography>
                    )}
                  </StyledTableCell>
                );
              })}
            </StyledTableRow>
          );
        })}
      </TableBody>
    </Table>
  );

  return (
    <Box data-testid="permission-matrix">
      {showSearch && (
        <TextField
          size="small"
          placeholder="Filter resources"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          sx={{ mb: 2, minWidth: 260 }}
          inputProps={{ 'aria-label': 'Filter resources' }}
        />
      )}
      {visibleGroups.map(({ group, builtIn, plugins }) => (
        <StyledPaper key={group} sx={{ mb: 3 }}>
          <Typography variant="headingMedium" sx={{ px: 2, pt: 2 }}>
            {group}
          </Typography>
          {builtIn.length > 0 && renderTable(builtIn, group)}
          {plugins.map((plugin) => (
            <Box key={plugin.plugin} data-testid={`matrix-plugin-${plugin.plugin}`}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, px: 2, pt: 2 }}>
                <Icon name="puzzle-piece" style={{ fontSize: 14, color: '#666' }} />
                <Typography variant="subtitle2">{plugin.label}</Typography>
                <Tooltip title="Contributed by an installed plugin. Holders of plugins:execute have every permission listed here; grant these to narrow access to this plugin.">
                  <span><Icon name="circle-info" style={{ fontSize: 14, color: '#999' }} /></span>
                </Tooltip>
              </Box>
              {renderTable(plugin.resources, plugin.plugin)}
            </Box>
          ))}
        </StyledPaper>
      ))}
      {visibleGroups.length === 0 && (
        <Typography color="text.secondary">No resources match “{search}”.</Typography>
      )}
    </Box>
  );
};

export default PermissionMatrix;
