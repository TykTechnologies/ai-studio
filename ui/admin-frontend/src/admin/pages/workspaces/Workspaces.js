import React, { useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import AddIcon from '@mui/icons-material/Add';
import {
  Alert,
  Chip,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import { TitleBox, ContentBox, PrimaryButton } from '../../styles/sharedStyles';
import EnterpriseFeatureBadge from '../../components/common/EnterpriseFeatureBadge';
import Can from '../../components/rbac/Can';
import { P } from '../../rbac/permissions';
import { listWorkspaces, getWorkspaceObjects } from '../../services/workspacesService';
import { totalObjects } from '../../components/workspaces/workspaceObjects';
import { isEnterpriseFeature, isPermissionDenied } from '../../utils/apiErrors';

/**
 * Workspaces list (Enterprise): name, description and how many objects each
 * owns; the Default first, marked. In Community Edition the API answers 402
 * and the page shows the upgrade prompt.
 */
const Workspaces = () => {
  const navigate = useNavigate();
  const [workspaces, setWorkspaces] = useState([]);
  const [counts, setCounts] = useState({});
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [enterpriseAvailable, setEnterpriseAvailable] = useState(true);

  const fetchWorkspaces = useCallback(async () => {
    try {
      setLoading(true);
      const list = await listWorkspaces();
      // The API lists the Default first; keep it there whatever it sends.
      const sorted = [...list].sort((a, b) => (b.attributes?.is_default ? 1 : 0) - (a.attributes?.is_default ? 1 : 0));
      setWorkspaces(sorted);
      setError('');
      const entries = await Promise.all(
        sorted.map((w) =>
          getWorkspaceObjects(w.id)
            .then((o) => [w.id, totalObjects(o?.counts)])
            .catch(() => [w.id, undefined])
        )
      );
      setCounts(Object.fromEntries(entries));
    } catch (err) {
      if (isEnterpriseFeature(err)) {
        setEnterpriseAvailable(false);
      } else if (isPermissionDenied(err)) {
        setError('Your role does not include access to Workspaces');
      } else {
        setError('Failed to load Workspaces');
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchWorkspaces();
  }, [fetchWorkspaces]);

  if (!enterpriseAvailable) {
    return (
      <>
        <TitleBox top="var(--studio-header-height)">
          <Typography variant="headingXLarge">Workspaces</Typography>
        </TitleBox>
        <ContentBox>
          <EnterpriseFeatureBadge
            feature="Workspaces"
            description="Group LLMs, Apps, tools, data sources and the rest into Workspaces, and give users and teams roles that apply only inside them."
          />
        </ContentBox>
      </>
    );
  }

  return (
    <>
      <TitleBox top="var(--studio-header-height)">
        <Typography variant="headingXLarge">Workspaces</Typography>
        <Can permission={P.WORKSPACES_WRITE}>
          <PrimaryButton variant="contained" startIcon={<AddIcon />} onClick={() => navigate('/admin/workspaces/new')}>
            Add Workspace
          </PrimaryButton>
        </Can>
      </TitleBox>
      <ContentBox>
        {error && <Alert severity="error">{error}</Alert>}
        {!loading && !error && (
          <Table data-testid="workspaces-table">
            <TableHead>
              <TableRow>
                <TableCell>Name</TableCell>
                <TableCell>Description</TableCell>
                <TableCell align="right">Objects</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {workspaces.map((w) => (
                <TableRow
                  key={w.id}
                  hover
                  sx={{ cursor: 'pointer' }}
                  data-testid={`workspace-row-${w.id}`}
                  onClick={() => navigate(`/admin/workspaces/${w.id}`)}
                >
                  <TableCell>
                    {w.attributes?.name}{' '}
                    {w.attributes?.is_default && <Chip size="small" label="Default" data-testid="default-badge" />}
                  </TableCell>
                  <TableCell>{w.attributes?.description || ''}</TableCell>
                  <TableCell align="right">{counts[w.id] ?? '—'}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </ContentBox>
    </>
  );
};

export default Workspaces;
