import React from 'react';
import { Box, Typography } from '@mui/material';
import Icon from '../../../components/common/Icon';

const CustomSelectBadge = ({ config }) => {
  return (
    <Box sx={{ 
      display: 'flex', 
      alignItems: 'center', 
      backgroundColor: config.bgColor,
      borderRadius: '6px',
      padding: '2px 8px',
      maxWidth: 'fit-content',
    }}>
      {
        config.icon && (
          <Icon 
            name={config.icon} 
            sx={{ 
              width: 16, 
              height: 16, 
              mr: 0.5,
              color: config.textColor
            }} 
          />
        )
      }
      <Typography 
        variant="bodySmallDefault" 
        sx={{ color: config.textColor }}
      >
        {config.text}
      </Typography>
    </Box>
  );
};

export default CustomSelectBadge;