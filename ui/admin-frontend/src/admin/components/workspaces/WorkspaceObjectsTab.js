import React from 'react';
import { Link as RouterLink } from 'react-router-dom';
import { Link, Table, TableBody, TableCell, TableHead, TableRow } from '@mui/material';
import { WORKSPACE_OBJECT_TYPES, workspaceListPath } from './workspaceObjects';

/**
 * How many objects of each type the Workspace owns, each linking to that
 * type's list filtered to the Workspace.
 */
const WorkspaceObjectsTab = ({ workspaceId, counts }) => (
  <Table data-testid="workspace-objects-table">
    <TableHead>
      <TableRow>
        <TableCell>Type</TableCell>
        <TableCell align="right">Objects</TableCell>
      </TableRow>
    </TableHead>
    <TableBody>
      {WORKSPACE_OBJECT_TYPES.filter((t) => counts && t.key in counts).map((t) => {
        const path = workspaceListPath(t, workspaceId);
        return (
          <TableRow key={t.key}>
            <TableCell>
              {path ? (
                <Link component={RouterLink} to={path} data-testid={`objects-link-${t.key}`}>
                  {t.label}
                </Link>
              ) : (
                t.label
              )}
            </TableCell>
            <TableCell align="right">{counts[t.key]}</TableCell>
          </TableRow>
        );
      })}
    </TableBody>
  </Table>
);

export default WorkspaceObjectsTab;
