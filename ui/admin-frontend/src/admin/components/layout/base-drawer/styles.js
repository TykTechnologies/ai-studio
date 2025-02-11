import { styled } from '@mui/material/styles';
import { Drawer as MuiDrawer, IconButton as MuiIconButton, ListItemButton } from '@mui/material';

export const StyledDrawer = styled(MuiDrawer, {
  shouldForwardProp: prop => !['width', 'open'].includes(prop),
})(({ theme, width }) => ({
  width,
  flexShrink: 0,
  '& .MuiDrawer-paper': {
    width,
    boxSizing: 'border-box',
    overflow: 'visible',
    transition: theme.transitions.create('width', {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.enteringScreen,
    }),
  },
}));

export const ToggleButton = styled(MuiIconButton)(({ theme }) => ({
  backgroundColor: '#ECECEF',
  alignSelf: 'flex-end',
  marginRight: theme.spacing(1),
  position: 'absolute',
  left: 'calc(100% - 20px)',
  top: '78px',
  zIndex: 9,
  '--IconButton-hoverBg': 'rgba(0, 0, 0, 0.04)',
  '&:hover': {
    backgroundColor: 'var(--IconButton-hoverBg)',
  },
}));

export const MenuList = styled('div')(({ theme, customMarginTop }) => ({
  marginTop: customMarginTop || 0,
}));
