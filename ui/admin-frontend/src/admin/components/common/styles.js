import { styled, useTheme } from "@mui/material/styles";
import { Dialog, DialogContent, DialogActions, Box, Typography, Tooltip } from "@mui/material";
import Icon from "../../../components/common/Icon";

export const StyledActionDialog = styled(Dialog)(({ theme }) => ({
  '& .MuiPaper-root': {
    border: "1px solid",
    borderColor: theme.palette.border.neutralDefault,
    borderRadius: 16,
    maxWidth: '95%',
    width: '95%',
    [theme.breakpoints.up('sm')]: {
        maxWidth: '85%',
        width: '85%',
    },
    [theme.breakpoints.up('md')]: {
        maxWidth: '80%',
        width: '80%',
    },
    [theme.breakpoints.up('lg')]: {
        maxWidth: '60%',
        width: '60%',
    },
    [theme.breakpoints.up('xl')]: {
        maxWidth: '50%',
        width: '50%',
    },
  }
}));

export const TitleBox = styled(Box)(({ theme }) => ({
  padding: theme.spacing(2)
}));

export const DialogDivider = styled(Box)(({ theme }) => ({
  margin: `0 ${theme.spacing(2)}`,
  borderBottom: "1px solid",
  borderColor: theme.palette.border.neutralDefault,
}));

export const StyledDialogContent = styled(DialogContent)(({ theme }) => ({
  padding: theme.spacing(2)
}));

export const StyledDialogActions = styled(DialogActions)(({ theme }) => ({
  justifyContent: "flex-end",
  padding: theme.spacing(2),
  gap: theme.spacing(1),
}));

export const MemberInfoContainer = styled(Box)(({ theme }) => ({
  display: 'flex',
  flexDirection: 'column',
  width: '100%',
  paddingRight: theme.spacing(1)
}));

export const TruncatedTypography = styled(Typography)(({ theme }) => ({
  whiteSpace: 'nowrap',
  overflow: 'hidden',
  textOverflow: 'ellipsis',
  width: '100%'
}));

export const ScrollContainer = styled(Box)(() => ({
  overflowY: 'auto',
  overflowX: 'hidden',
  height: '100%',
  flex: 1,
}));

const tooltipPopupSx = (theme) => ({
  backgroundColor: theme.palette.background.buttonPrimaryDefaultHover, 
  borderRadius: '8px',
  '& .MuiTypography-root': { 
    color: 'white',
  },
});

const tooltipArrowSx = (theme) => ({
  color: theme.palette.background.buttonPrimaryDefaultHover,
});

export const StyledTooltip = (props) => {
  const theme = useTheme();
  const { open, onClose, onClick, ...rest } = props;

  return (
    <Tooltip
      {...rest}
      open={open}
      onClose={onClose}
      onClick={onClick}
      arrow
      componentsProps={{
        tooltip: {
          sx: tooltipPopupSx(theme),
        },
        arrow: {
          sx: tooltipArrowSx(theme),
        },
      }}
    />
  );
};

export const TooltipIcon = styled(Icon)(({ theme }) => ({
  color: theme.palette.border.neutralPressed,
  width: 16,
  height: 16,
  cursor: 'pointer',
}));