import React from 'react';
import {
  Box,
  Typography,
  IconButton,
} from '@mui/material';
import Icon from '../../../components/common/Icon';
import CloseIcon from '@mui/icons-material/Close';

const Banner = ({
  title,
  message,
  onClose,
  linkText,
  linkUrl,
  showCloseButton = true,
  horizontalLayout = false,
  iconName,
  iconColor,
  borderColor,
  backgroundColor,
  titleColor,
  button = null,
  sx = {},
}) => {
  return (
    <Box
      sx={{
        mb: 3,
        p: 2,
        border: '1px solid',
        borderColor: borderColor,
        bgcolor: backgroundColor,
        borderRadius: 1,
        display: 'flex',
        alignItems: 'flex-start',
        gap: 1,
        ...sx
      }}
    >
      <Icon
        name={iconName}
        sx={{
          width: 16,
          height: 16,
          mt: 0.3,
          color: iconColor
        }}
      />
      
      <Box sx={{
        display: 'flex',
        flexDirection: horizontalLayout ? 'row' : 'column',
        alignItems: horizontalLayout ? 'center' : 'flex-start',
        px: 1,
        py: 0,
        flexGrow: 1
      }}>
        <Typography
          variant="headingSmall"
          sx={{
            color: titleColor,
            mb: horizontalLayout ? 0 : 0.5,
            mr: horizontalLayout ? 0.5 : 0
          }}
        >
          {title}
        </Typography>
        
        {message && (
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
        )}
        
        {!horizontalLayout && button && (
          <Box sx={{ mt: 0.5 }}>
            {button}
          </Box>
        )}
      </Box>
      
      {horizontalLayout && button && (
        <Box sx={{ alignSelf: 'center', ml: 2 }}>
          {button}
        </Box>
      )}
      
      {showCloseButton && onClose && (
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
      )}
    </Box>
  );
};

export default Banner;