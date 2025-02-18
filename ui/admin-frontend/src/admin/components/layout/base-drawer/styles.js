import { styled } from '@mui/material/styles';
import { Drawer as MuiDrawer, IconButton as MuiIconButton, ListItemButton, ListItemIcon as MuiListItemIcon } from '@mui/material';

export const StyledDrawer = styled(MuiDrawer, {
  shouldForwardProp: prop => !['width', 'open', 'divider'].includes(prop),
})(({ theme, width, open }) => ({
  width,
  flexShrink: 0,
  '& .MuiDrawer-paper': {
    width,
    boxSizing: 'border-box',
    overflow: 'visible',
    background: theme.palette.custom.white,
    transition: theme.transitions.create('width', {
      easing: theme.transitions.easing.sharp,
      duration: theme.transitions.duration.enteringScreen,
    }),
    '& .MuiList-root': {
      padding: 0,
      overflow: 'hidden',
      position: 'relative',
      '& .MuiListItemText-root, & .MuiSvgIcon-root:not(.MuiListItemIcon-root svg)': {
        opacity: open ? 1 : 0,
        transition: theme.transitions.create('opacity', {
          easing: theme.transitions.easing.sharp,
          duration: theme.transitions.duration.enteringScreen,
        }),
        pointerEvents: open ? 'auto' : 'none',
      },
    },
    '&::before': {
      content: '""',
      position: 'absolute',
      top: 0,
      left: 0,
      width: '60px',
      height: '100%',
      background: `linear-gradient(178.19deg, ${theme.palette.text.default} 38.77%, ${theme.palette.primary.main} 92.63%, ${theme.palette.custom.purpleDark} 99.36%, ${theme.palette.custom.purpleLight} 106.1%)`,
      zIndex: -2,
    },
  },
}));

export const ToggleButton = styled(MuiIconButton)(({ theme }) => ({
  backgroundColor: theme.palette.background.defaultSubdued,
  position: 'absolute',
  right: '-12px',
  top: '112px',
  zIndex: 1201,
  minWidth: '24px',
  minHeight: '24px',
  width: '24px',
  height: '24px',
  padding: 0,
  borderRadius: '50%',
  border: `1px solid ${theme.palette.border.neutralDefault}`,
  '& .MuiSvgIcon-root': {
    fontSize: '20px',
    width: '20px',
    height: '20px',
    display: 'block',
    margin: 'auto',
  },
  '&:hover': {
    backgroundColor: '#E6E6EA',
  }
}));

export const MenuList = styled('div')(({ customMarginTop }) => ({
  position: 'relative',
  marginTop: customMarginTop || 0,
  zIndex: 1,
  overflow: 'hidden',
}));

export const ParentListItem = styled(ListItemButton, {
  shouldForwardProp: prop => !['isFirstItem', 'open'].includes(prop),
})(({ theme, isFirstItem }) => ({
  padding: '8px 16px',
  paddingLeft: '60px',
  height: '60px',
  position: 'relative',
  background: 'transparent !important',
  borderImage: isFirstItem ? 'none' : `linear-gradient(to right, transparent 60px, ${theme.palette.border.neutralDefault} 60px) 1`,
  borderWidth: '1px 0 0 0',
  borderStyle: 'solid',
  '&:hover, &:active, &.Mui-selected, &.Mui-focusVisible': {
    background: 'transparent !important',
  },
  '& .MuiListItemText-root': {
    marginLeft: '16px',
    position: 'relative',
    zIndex: 2,
    '& .MuiTypography-root': {
      fontFamily: 'Inter-Medium',
      fontSize: '14px',
      lineHeight: '20px',
      color: theme.palette.text.default,
    },
  },
  '&:hover, &:active, &.Mui-selected, &.active': {
    '&::before': {
      content: '""',
      position: 'absolute',
      top: 0,
      left: 0,
      width: '58px',
      height: '100%',
      background: theme.palette.background.surfaceDefaultBoldest,
      zIndex: 0,
    },
    '&::after': {
      content: '""',
      position: 'absolute',
      top: 0,
      left: '58px',
      width: '2px',
      height: '100%',
      background: `linear-gradient(163.33deg, ${theme.palette.primary.main} 46.22%, ${theme.palette.custom.purpleExtraDark} 161.35%)`,
      zIndex: 0,
    },
    '& .MuiListItemText-root .MuiTypography-root': {
      fontFamily: 'Inter-Bold',
      color: theme.palette.text.dark,
    },
    '& .MuiListItemIcon-root': {
      color: theme.palette.primary.main,
    },
  },
}));

export const SubListItem = styled(ListItemButton, {
  shouldForwardProp: prop => !['depth', 'rootParentId', 'itemId', 'open', 'hasSubItems'].includes(prop),
})(({ theme, depth = 1, rootParentId, itemId, hasSubItems, open }) => ({
  padding: '8px 16px',
  paddingLeft: depth === 1 ? '85px' : '95px',
  height: '34px',
  position: 'relative',
  background: 'transparent !important',
  '&:hover, &:active, &.Mui-selected, &.Mui-focusVisible': {
    background: 'transparent !important',
  },
  '& .MuiListItemText-root': {
    overflow: 'hidden',
    '& .MuiTypography-root': {
      fontFamily: 'Inter-Medium',
      fontSize: '14px',
      lineHeight: '20px',
      color: theme.palette.text.defaultSubdued,
      position: 'relative',
      zIndex: 2,
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis'
    }
  },
  '&::after': {
    content: '""',
    position: 'absolute',
    top: 0,
    right: 0,
    width: '2px',
    height: '100%',
    background: 'transparent',
    transition: 'background-color 0.2s ease',
    zIndex: 1,
  },
  '&:hover': {
    '& .MuiListItemText-root .MuiTypography-root': {
      color: theme.palette.text.default,
    },
    '&::before': {
      content: '""',
      position: 'absolute',
      top: 0,
      left: '60px',
      right: 0,
      height: '100%',
      background: theme.palette.background.surfaceDefaultHover,
      opacity: open ? 1 : 0,
      transition: theme.transitions.create('opacity', {
        easing: theme.transitions.easing.sharp,
        duration: theme.transitions.duration.enteringScreen,
      }),
      zIndex: 0,
    },
    '&::after': {
      background: theme.palette.primary.main,
      zIndex: 1,
    },
  },
  '&.active, &.Mui-selected': {
    '& .MuiListItemText-root .MuiTypography-root': {
      color: theme.palette.text.default,
    },
    '&::before': {
      content: '""',
      position: 'absolute',
      top: 0,
      left: '60px',
      right: 0,
      height: '100%',
      background: depth === 1 
        ? (hasSubItems && rootParentId
            ? theme.palette.background.surfaceDefaultHover
            : theme.palette.background.surfaceDefaultSelected)
        : theme.palette.background.surfaceDefaultSelected,
      opacity: open ? 1 : 0,
      transition: theme.transitions.create('opacity', {
        easing: theme.transitions.easing.sharp,
        duration: theme.transitions.duration.enteringScreen,
      }),
      zIndex: 0,
    },
    '&::after': {
      background: theme.palette.primary.main,
      zIndex: 1,
    },
  },
}));

export const ListItemIcon = styled(MuiListItemIcon)(({ theme }) => ({
  position: 'absolute',
  left: 0,
  minWidth: '60px',
  height: '100%',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  color: theme.palette.background.default,
  zIndex: 2,
}));
