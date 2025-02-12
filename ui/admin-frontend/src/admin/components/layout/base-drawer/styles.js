import { styled } from '@mui/material/styles';
import { Drawer as MuiDrawer, IconButton as MuiIconButton, ListItemButton, ListItemIcon as MuiListItemIcon } from '@mui/material';

export const StyledDrawer = styled(MuiDrawer, {
  shouldForwardProp: prop => !['width', 'open'].includes(prop),
})(({ theme, width }) => ({
  width,
  flexShrink: 0,
  '& .MuiDrawer-paper': {
    width,
    boxSizing: 'border-box',
    overflow: 'visible',
    background: '#fff',
    transition: theme.transitions.create('width', {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.enteringScreen,
    }),
    '&::before': {
      content: '""',
      position: 'absolute',
      top: 0,
      left: 0,
      width: '60px',
      height: '100%',
      background: 'linear-gradient(178.19deg, #03031C 38.77%, #23E2C2 92.63%, #8438FA 99.36%, #B421FA 106.1%)',
      zIndex: 0,
    },
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
  position: 'relative',
  marginTop: customMarginTop || 0,
  zIndex: 1,
}));

export const StyledListItem = styled(ListItemButton)(({ theme, depth = 0 }) => ({
  padding: '8px 16px',
  paddingLeft: depth === 0 ? '60px' : '76px',
  height: '60px',
  borderBottom: '1px solid #D8D8DF',
  '& .MuiListItemText-root': {
    marginLeft: '16px',
  },
  '&:hover, &.Mui-selected': {
    background: depth === 0 ? 
      'linear-gradient(90deg, rgba(35, 226, 194, 0.2) 0px, rgba(35, 226, 194, 0.2) 58px, #23E2C2 58px, #23E2C2 60px, #fff 60px)' : 
      'transparent',
  },
  '& .MuiListItemIcon-root': {
    color: '#fff',
  },
}));

export const ListItemIcon = styled(MuiListItemIcon)({
  position: 'absolute',
  left: 0,
  minWidth: '60px',
  justifyContent: 'center',
  color: '#fff',
});
