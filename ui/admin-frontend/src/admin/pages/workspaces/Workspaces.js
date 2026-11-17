import React, { useCallback, useEffect, useState } from 'react';
import { Alert, Chip, List, ListItem, ListItemText, Typography } from '@mui/material';
import { TitleBox, ContentBox } from '../../styles/sharedStyles';
import EnterpriseFeatureBadge from '../../components/common/EnterpriseFeatureBadge';
import { listWorkspaces } from '../../services/workspacesService';
import { isEnterpriseFeature, isPermissionDenied } from '../../utils/apiErrors';

/**
 * Workspaces list (Enterprise). In Community Edition the API answers 402
 * and the page shows the upgrade prompt, as the Roles page does. The full
 * list, form and detail pages come with Workspaces E2.
 */
const Workspaces = () => {
  const [workspaces, setWorkspaces] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [enterpriseAvailable, setEnterpriseAvailable] = useState(true);

  const fetchWorkspaces = useCallback(async () => {
    try {
      setLoading(true);
      setWorkspaces(await listWorkspaces());
      setError('');
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

  return (
    <>
      <TitleBox top="var(--studio-header-height)">
        <Typography variant="headingXLarge">Workspaces</Typography>
      </TitleBox>
      <ContentBox>
        {!enterpriseAvailable ? (
          <EnterpriseFeatureBadge
            feature="Workspaces"
            description="Group LLMs, Apps, tools, data sources and the rest into Workspaces, and give users and teams roles that apply only inside them."
          />
        ) : (
          <>
            {error && <Alert severity="error">{error}</Alert>}
            {!loading && !error && (
              <List data-testid="workspaces-list">
                {workspaces.map((w) => (
                  <ListItem key={w.id} disableGutters>
                    <ListItemText primary={w.attributes?.name} secondary={w.attributes?.description || null} />
                    {w.attributes?.is_default && <Chip size="small" label="Default" />}
                  </ListItem>
                ))}
              </List>
            )}
          </>
        )}
      </ContentBox>
    </>
  );
};

export default Workspaces;
