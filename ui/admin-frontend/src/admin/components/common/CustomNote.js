import React from 'react';
import {
  Box,
  Typography,
} from '@mui/material';
import Icon from '../../../components/common/Icon';

const CustomNote = ({ title, message }) => {
  return (
    <Box
      sx={{
        mb: 3,
        p: 2,
        border: '2px solid',
        borderColor: 'border.informativeDefaultSubdued',
        bgcolor: 'background.surfaceInformativeDefault',
        borderRadius: 1,
        display: 'flex',
        alignItems: 'flex-start',
        gap: 1,
      }}
    >
      <Icon
        name="circle-info"
        sx={{
          width: 16,
          height: 16,
          mt: 0.3,
          color: theme => theme.palette.text.linkDefault
        }}
      />
      
      <Box sx={{ display: 'flex', flexDirection: 'column', px: 1, py: 0 }}>
        {title && (
          <Typography
            variant="bodyLargeBold"
            sx={{ color: 'text.linkDefault', mb: 0.5 }}
          >
            {title}
          </Typography>
        )}
        
        <Typography
          variant="bodyLargeDefault"
          sx={{ color: 'text.defaultSubdued' }}
        >
          {message}
        </Typography>
      </Box>
    </Box>
  );
};

export default CustomNote;