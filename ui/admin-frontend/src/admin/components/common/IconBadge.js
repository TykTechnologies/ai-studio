import React from 'react';
import { Box, styled, useTheme, SvgIcon } from '@mui/material';
import Icon from '../../../components/common/Icon';

const BadgeContainer = styled(Box)(({ theme }) => ({
  width: '42px',
  height: '42px',
  minWidth: '42px',
  minHeight: '42px',
  maxWidth: '42px',
  maxHeight: '42px',
  borderRadius: '50%',
  background: theme.palette.background.surfaceNeutralHover,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  margin: 0,
  position: 'relative',
}));

const StyledIcon = styled(Icon)(({ theme }) => ({
  width: '24px',
  height: '24px',
  color: 'transparent',
  '& path': {
    fill: `url(#gradient-${theme.palette.primary.main.replace('#', '')})`,
  }
}));

const IconBadge = ({ iconName }) => {
  const theme = useTheme();
  const gradientId = `gradient-${theme.palette.primary.main.replace('#', '')}`;
  
  return (
    <BadgeContainer>
      <SvgIcon style={{ position: 'absolute', width: 0, height: 0 }}>
        <defs>
          <linearGradient id={gradientId} gradientTransform="rotate(91.42)">
            <stop offset="0%" stopColor={theme.palette.primary.main} />
            <stop offset="100%" stopColor={theme.palette.custom.purpleExtraDark} />
          </linearGradient>
        </defs>
      </SvgIcon>
      <StyledIcon name={iconName} />
    </BadgeContainer>
  );
};



export default IconBadge;