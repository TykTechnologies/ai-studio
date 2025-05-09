import React from 'react';
import Banner from './Banner';
import { SecondaryOutlineButton } from '../../styles/sharedStyles';

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
  sx = {} 
}) => {
  const button = buttonText && onButtonClick ? (
    <SecondaryOutlineButton 
      onClick={onButtonClick} 
      size="small"
    >
      {buttonText}
    </SecondaryOutlineButton>
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