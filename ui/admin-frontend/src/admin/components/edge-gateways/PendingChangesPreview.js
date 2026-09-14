import React from 'react';
import PropTypes from 'prop-types';
import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Chip,
  CircularProgress,
  Link,
  List,
  ListItem,
  ListItemText,
  Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import { Link as RouterLink } from 'react-router-dom';
import { changeLink, groupChanges, relativeTime, summarizePending } from './pendingChanges';

const CHANGE_COLOR = { created: 'success', updated: 'info', deleted: 'default' };

const Dot = () => (
  <Typography component="span" variant="body2" color="text.secondary" aria-hidden="true">
    {'·'}
  </Typography>
);

/**
 * The list of changes for one namespace: a summary line, then the changes
 * grouped by type ("LLM providers", "Apps", ...), each as
 * "name - updated - 5 minutes ago" linking to the object's admin page where
 * one exists.
 */
export const NamespaceChanges = ({ namespace, state, onNavigate }) => {
  const { loading, error, data } = state || {};

  if (loading) {
    return (
      <Box
        display="flex"
        alignItems="center"
        gap={1.5}
        sx={{ py: 1 }}
        data-testid={`pending-changes-loading-${namespace}`}
      >
        <CircularProgress size={16} />
        <Typography variant="body2" color="text.secondary">
          Working out what has changed...
        </Typography>
      </Box>
    );
  }

  if (error) {
    return (
      <Alert severity="warning" sx={{ py: 0.5 }}>
        Could not load the list of changes ({error}). You can still push; the
        gateways will receive the current configuration.
      </Alert>
    );
  }

  if (!data) return null;

  const groups = groupChanges(data.changes);
  const shown = (data.changes || []).length;
  const more = Math.max(0, (data.total || 0) - shown);

  return (
    <Box data-testid={`pending-changes-${namespace}`}>
      <Typography
        variant="body2"
        sx={{ fontWeight: data.total > 0 ? 600 : 400, mb: groups.length ? 1 : 0 }}
        data-testid="pending-changes-summary"
      >
        {summarizePending(data)}
      </Typography>

      {groups.map((group) => (
        <Box key={group.type} sx={{ mb: 1 }} data-testid={`pending-changes-group-${group.type}`}>
          <Typography variant="overline" color="text.secondary" component="div" sx={{ lineHeight: 1.8 }}>
            {group.label}
          </Typography>
          <List dense disablePadding>
            {group.changes.map((change, index) => {
              const href = changeLink(change);
              const name = change.name || `#${change.id}`;
              return (
                <ListItem
                  key={`${change.type}-${change.id ?? index}-${change.at ?? index}`}
                  disableGutters
                  sx={{ py: 0.25 }}
                >
                  <ListItemText
                    primaryTypographyProps={{ variant: 'body2', component: 'div' }}
                    primary={
                      <Box component="span" display="flex" alignItems="center" gap={1} flexWrap="wrap">
                        {href ? (
                          <Link component={RouterLink} to={href} onClick={onNavigate} underline="hover">
                            {name}
                          </Link>
                        ) : (
                          <span>{name}</span>
                        )}
                        <Dot />
                        <Chip
                          label={change.change}
                          size="small"
                          variant="outlined"
                          color={CHANGE_COLOR[change.change] || 'default'}
                          sx={{ height: 20 }}
                        />
                        {change.at && (
                          <>
                            <Dot />
                            <Typography component="span" variant="body2" color="text.secondary">
                              {relativeTime(change.at)}
                            </Typography>
                          </>
                        )}
                      </Box>
                    }
                  />
                </ListItem>
              );
            })}
          </List>
        </Box>
      ))}

      {more > 0 && (
        <Typography variant="body2" color="text.secondary">
          and {more} more
        </Typography>
      )}
    </Box>
  );
};

NamespaceChanges.propTypes = {
  namespace: PropTypes.string.isRequired,
  state: PropTypes.shape({
    loading: PropTypes.bool,
    error: PropTypes.string,
    data: PropTypes.object,
  }),
  onNavigate: PropTypes.func,
};

/**
 * What a push will send. One namespace renders its list directly; several
 * render one collapsible block each, open when the namespace has something
 * to push.
 */
const PendingChangesPreview = ({ namespaces, byNamespace, onNavigate }) => {
  if (!namespaces || namespaces.length === 0) return null;

  if (namespaces.length === 1) {
    const ns = namespaces[0];
    return (
      <Box sx={{ mb: 2 }} data-testid="pending-changes-preview">
        <Typography variant="subtitle2" gutterBottom>
          What will be pushed
        </Typography>
        <NamespaceChanges namespace={ns} state={byNamespace[ns]} onNavigate={onNavigate} />
      </Box>
    );
  }

  return (
    <Box sx={{ mb: 2 }} data-testid="pending-changes-preview">
      <Typography variant="subtitle2" gutterBottom>
        What will be pushed
      </Typography>
      {namespaces.map((ns) => {
        const state = byNamespace[ns] || {};
        const total = state.data?.total ?? null;
        return (
          <Accordion
            key={ns}
            defaultExpanded={total === null || total > 0}
            disableGutters
            elevation={0}
            sx={{ border: 1, borderColor: 'divider', '&:before': { display: 'none' }, mb: 1 }}
          >
            <AccordionSummary expandIcon={<ExpandMoreIcon />}>
              <Box display="flex" alignItems="center" gap={1}>
                <Typography variant="body2" fontWeight={600}>
                  {ns === 'global' ? 'Global' : ns}
                </Typography>
                {total !== null && (
                  <Chip size="small" label={`${total} change${total === 1 ? '' : 's'}`} sx={{ height: 20 }} />
                )}
              </Box>
            </AccordionSummary>
            <AccordionDetails sx={{ pt: 0 }}>
              <NamespaceChanges namespace={ns} state={state} onNavigate={onNavigate} />
            </AccordionDetails>
          </Accordion>
        );
      })}
    </Box>
  );
};

PendingChangesPreview.propTypes = {
  namespaces: PropTypes.arrayOf(PropTypes.string).isRequired,
  byNamespace: PropTypes.object.isRequired,
  onNavigate: PropTypes.func,
};

export default PendingChangesPreview;
