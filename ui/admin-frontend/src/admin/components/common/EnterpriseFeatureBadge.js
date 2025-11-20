import React from 'react';
import { Box, Typography, Paper, Button } from '@mui/material';
import Icon from '../../../components/common/Icon';

const EnterpriseFeatureBadge = ({
  feature = "Enterprise Feature",
  description = "This feature is only available in the Enterprise Edition.",
  showUpgradeButton = true
}) => {
  return (
    <Paper
      elevation={0}
      sx={{
        p: 4,
        textAlign: 'center',
        backgroundColor: '#f5f5f5',
        border: '2px dashed #ddd',
        borderRadius: 2,
        maxWidth: 600,
        mx: 'auto',
        mt: 4
      }}
    >
      <Box sx={{ mb: 2 }}>
        <Icon
          name="lock"
          style={{
            fontSize: 48,
            color: '#666',
            opacity: 0.5
          }}
        />
      </Box>

      <Typography
        variant="h5"
        gutterBottom
        sx={{ fontWeight: 600, color: '#333' }}
      >
        {feature}
      </Typography>

      <Typography
        variant="body1"
        color="text.secondary"
        sx={{ mb: 3 }}
      >
        {description}
      </Typography>

      {showUpgradeButton && (
        <Button
          variant="contained"
          color="primary"
          href="https://tyk.io/enterprise"
          target="_blank"
          rel="noopener noreferrer"
          sx={{ textTransform: 'none' }}
        >
          Learn More About Enterprise
        </Button>
      )}
    </Paper>
  );
};

export default EnterpriseFeatureBadge;
