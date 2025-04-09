import React from 'react';
import {
  Box,
  Typography,
  IconButton,
} from '@mui/material';
import Icon from '../../../components/common/Icon';
import CloseIcon from '@mui/icons-material/Close';

const SuccessBanner = ({ title, message, onClose, linkText, linkUrl }) => {
  return (
    <Box
      sx={{
        mb: 3,
        p: 2,
        border: '1px solid',
        borderColor: 'border.successDefaultSubdued',
        bgcolor: 'background.surfaceSuccessDefault',
        borderRadius: 1,
        display: 'flex',
        alignItems: 'flex-start',
        gap: 1,
        
      }}
    >
      <Icon
        name="hexagon-check"
        sx={{
          width: 16,
          height: 16,
          mt: 0.3,
          color: theme => theme.palette.background.iconSuccessDefault
        }}
      />
      
      <Box sx={{ display: 'flex', flexDirection: 'column', px: 1, py: 0 }}>
        <Typography
          variant="headingSmall"
          sx={{ color: 'text.successDefault', mb: 0.5 }}
        >
          {title}
        </Typography>
        
        <Typography
          variant="bodyMediumDefault"
          sx={{ color: 'text.defaultSubdued' }}
        >
          {message}
          {linkText && linkUrl && (
            <>
              {' '}
              <Typography
                component="a"
                href={linkUrl}
                variant="bodyMediumDefault"
                sx={{ color: 'primary.main', textDecoration: 'none' }}
              >
                {linkText}
              </Typography>
            </>
          )}
        </Typography>
      </Box>
      
      <IconButton
        onClick={onClose}
        size="small"
        sx={{
          p: 0,
          ml: 'auto'
        }}
      >
        <CloseIcon fontSize="small" />
      </IconButton>
    </Box>
  );
};

export default SuccessBanner;