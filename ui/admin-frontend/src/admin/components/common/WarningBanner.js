import React from 'react';
import { Box } from '@mui/material';
import Banner from './Banner';
import { SecondaryOutlineButton, PrimaryButton } from '../../styles/sharedStyles';

const WarningBanner = ({ 
  title, 
  message, 
  onClose, 
  linkText, 
  linkUrl, 
  showCloseButton = true, 
  horizontalLayout = false, 
  buttonText = null,
  onButtonClick = null,
  // A banner that reports a problem should be able to carry the action that
  // resolves it, not only a link to the page where the action lives.
  primaryButtonText = null,
  onPrimaryButtonClick = null,
  sx = {} 
}) => {
  const secondary = buttonText && onButtonClick ? (
    <SecondaryOutlineButton 
      onClick={onButtonClick} 
      size="small"
    >
      {buttonText}
    </SecondaryOutlineButton>
  ) : null;

  const primary = primaryButtonText && onPrimaryButtonClick ? (
    <PrimaryButton onClick={onPrimaryButtonClick} size="small">
      {primaryButtonText}
    </PrimaryButton>
  ) : null;

  const button = primary || secondary ? (
    <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
      {primary}
      {secondary}
    </Box>
  ) : null;

  return (
    <Banner
      title={title}
      message={message}
      onClose={onClose}
      linkText={linkText}
      linkUrl={linkUrl}
      showCloseButton={showCloseButton}
      horizontalLayout={horizontalLayout}
      iconName="triangle-exclamation"
      iconColor="background.iconWarningDefault"
      borderColor="border.warningDefaultSubdued"
      backgroundColor="background.surfaceWarningDefault"
      titleColor="text.warningDefault"
      button={button}
      sx={sx}
    />
  );
};

export default WarningBanner;