import React from 'react';
import Banner from './Banner';

const SuccessBanner = ({
  title,
  message,
  onClose,
  linkText,
  linkUrl,
  showCloseButton = true,
  horizontalLayout = false,
  sx = {}
}) => {
  return (
    <Banner
      title={title}
      message={message}
      onClose={onClose}
      linkText={linkText}
      linkUrl={linkUrl}
      showCloseButton={showCloseButton}
      horizontalLayout={horizontalLayout}
      iconName="hexagon-check"
      iconColor={theme => theme.palette.background.iconSuccessDefault}
      borderColor="border.successDefaultSubdued"
      backgroundColor="background.surfaceSuccessDefault"
      titleColor="text.successDefault"
      sx={sx}
    />
  );
};

export default SuccessBanner;