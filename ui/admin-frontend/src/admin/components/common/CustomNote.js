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
        p: { xs: 1.5, sm: 2 },
        border: '2px solid',
        borderColor: 'border.informativeDefaultSubdued',
        bgcolor: 'background.surfaceInformativeDefault',
        borderRadius: 1,
        display: 'flex',
        alignItems: 'flex-start',
        gap: { xs: 0.5, sm: 1 },
      }}
    >
      <Icon
        name="circle-info"
        sx={{
          width: { xs: 14, sm: 16 },
          height: { xs: 14, sm: 16 },
          mt: 0.3,
          color: theme => theme.palette.text.linkDefault
        }}
      />
      
      <Box sx={{
        display: 'flex',
        flexDirection: 'column',
        px: { xs: 0.5, sm: 1 },
        py: 0
      }}>
        {title && (
          <Typography
            variant="bodyLargeBold"
            sx={{
              color: 'text.linkDefault',
              mb: 0.5,
              fontSize: { xs: '0.875rem', sm: 'inherit' }
            }}
          >
            {title}
          </Typography>
        )}
        
        <Typography
          variant="bodyLargeDefault"
          sx={{
            color: 'text.defaultSubdued',
            fontSize: { xs: '0.875rem', sm: 'inherit' }
          }}
        >
          {message}
        </Typography>
      </Box>
    </Box>
  );
};

export default CustomNote;