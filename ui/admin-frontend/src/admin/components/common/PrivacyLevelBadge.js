import React from 'react';
import { Box, Typography } from '@mui/material';
import Icon from '../../../components/common/Icon';

const PrivacyLevelBadge = ({ level }) => {
  const badgeConfig = {
    public: {
      icon: 'unlock',
      text: 'Public',
      textColor: 'text.successDefault',
      bgColor: 'border.successDefaultSubdued'
    },
    internal: {
      icon: 'lock',
      text: 'Internal',
      textColor: 'text.warningDefault',
      bgColor: 'border.warningDefaultSubdued'
    },
    confidential: {
      icon: 'lock-keyhole',
      text: 'Confidential',
      textColor: 'border.criticalHover',
      bgColor: 'border.criticalDefaultSubdue'
    },
    restricted: {
      icon: 'shield-keyhole',
      text: 'Restricted',
      textColor: 'background.surfaceCriticalDefault',
      bgColor: 'background.buttonPrimaryDefault'
    }
  };

  const config = badgeConfig[level] || badgeConfig.public;

  return (
    <Box sx={{ 
      display: 'flex', 
      alignItems: 'center', 
      backgroundColor: config.bgColor,
      borderRadius: '6px',
      padding: '2px 8px'
    }}>
      <Icon 
        name={config.icon} 
        sx={{ 
          width: 16, 
          height: 16, 
          mr: 0.5,
          color: config.textColor
        }} 
      />
      <Typography 
        variant="bodySmallDefault" 
        sx={{ color: config.textColor }}
      >
        {config.text}
      </Typography>
    </Box>
  );
};

export default PrivacyLevelBadge;