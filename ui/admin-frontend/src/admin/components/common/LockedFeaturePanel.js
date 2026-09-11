import React from 'react';
import { Box, Typography, Paper, Button } from '@mui/material';
import Icon from '../../../components/common/Icon';

/**
 * A centred, dashed placeholder for something the user cannot use right now:
 * an Enterprise-only feature, or a page their role does not allow.
 */
const LockedFeaturePanel = ({
  icon = 'lock',
  title,
  description,
  action = null,
  testId,
}) => (
  <Paper
    elevation={0}
    data-testid={testId}
    sx={{
      p: 4,
      textAlign: 'center',
      backgroundColor: '#f5f5f5',
      border: '2px dashed #ddd',
      borderRadius: 2,
      maxWidth: 600,
      mx: 'auto',
      mt: 4,
    }}
  >
    <Box sx={{ mb: 2 }}>
      <Icon name={icon} style={{ fontSize: 48, color: '#666', opacity: 0.5 }} />
    </Box>

    <Typography variant="h5" gutterBottom sx={{ fontWeight: 600, color: '#333' }}>
      {title}
    </Typography>

    {description && (
      <Typography variant="body1" color="text.secondary" sx={{ mb: 3 }}>
        {description}
      </Typography>
    )}

    {action}
  </Paper>
);

export const LockedFeatureAction = ({ href, to, onClick, children }) => (
  <Button
    variant="contained"
    color="primary"
    href={href}
    onClick={onClick}
    target={href ? '_blank' : undefined}
    rel={href ? 'noopener noreferrer' : undefined}
    sx={{ textTransform: 'none' }}
  >
    {children}
  </Button>
);

export default LockedFeaturePanel;
