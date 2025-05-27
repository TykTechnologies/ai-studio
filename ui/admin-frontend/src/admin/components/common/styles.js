import { styled } from "@mui/material/styles";
import { Dialog, DialogContent, DialogActions, Box, Typography } from "@mui/material";

export const StyledActionDialog = styled(Dialog)(({ theme }) => ({
  '& .MuiPaper-root': {
    border: "1px solid",
    borderColor: theme.palette.border.neutralDefault,
    borderRadius: 16,
    maxWidth: '95%',
    [theme.breakpoints.up('sm')]: {
      maxWidth: '85%',
    },
    [theme.breakpoints.up('md')]: {
      maxWidth: '80%',
    },
    [theme.breakpoints.up('lg')]: {
      maxWidth: '60%',
    },
    [theme.breakpoints.up('xl')]: {
      maxWidth: '50%',
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