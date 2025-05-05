// midsommar/ui/admin-frontend/src/styles/sharedStyles.js
import { styled } from "@mui/material/styles";
import {
  Paper,
  Box,
  Typography,
  Button,
  TableCell,
  TableRow,
  Dialog,
  DialogTitle,
  Accordion,
  DialogContent,
  TextField,
  Select,
  FormControl,
  InputLabel,
} from "@mui/material";
import IconButton from "@mui/material/IconButton";
import { NavLink } from "react-router-dom";

export const StyledPaper = styled(Paper)(({ theme }) => ({
  backgroundColor: theme.palette.background.paper,
  borderRadius: 8,
  border: `1px solid rgba(0, 0, 0, 0.12)`,
  boxShadow: "none",
  overflow: "hidden",
}));

export const TitleBox = styled(Box)(({ theme, top = '0px' }) => ({
  position: 'sticky',
  top: top,
  zIndex: 1000,
  borderBottom: `1px solid rgba(0, 0, 0, 0.12)`,
  backgroundColor: theme.palette.background.paper,
  color: theme.palette.text.primary,
  padding: theme.spacing(3),
  display: 'flex',
  justifyContent: 'space-between',
  alignItems: 'center',
}));

export const ContentBox = styled(Box)(({ theme }) => ({
  padding: theme.spacing(3),
  maxWidth: '100%',
  overflowX: 'hidden',
}));

export const StyledTableHeaderCell = styled(TableCell)(({ theme }) => ({
  backgroundColor: theme.palette.background.neutralDefault,
  fontFamily: 'Inter-Semibold',
  color: theme.palette.text.primary,
  borderBottom: `1px solid ${theme.palette.border.neutralDefault}`,
}));

export const StyledTableCell = styled(TableCell)(({ theme }) => ({
  borderBottom: `1px solid ${theme.palette.border.neutralDefault}`,
  color: theme.palette.text.primary,
}));

export const StyledTableRow = styled(TableRow)(({ theme }) => ({
  "&:nth-of-type(odd)": {
    backgroundColor: "transparent",
  },
  "& td": {
    borderBottom: `1px solid ${theme.palette.border.neutralDefault}`
  },
  "&:last-child td": {
    borderBottom: "none"
  },
  "&:hover": {
    backgroundColor: theme.palette.background.secondaryExtraLight
  }
}));

export const FieldLabel = styled(Typography)(({ theme }) => ({
  fontWeight: "bold",
  color: theme.palette.text.secondary,
}));

export const FieldValue = styled(Typography)(({ theme }) => ({
  color: theme.palette.text.primary,
}));

export const PrimaryButton = styled(Button)(({ theme }) => ({
  position: 'relative',
  borderRadius: 20,
  padding: '8px 16px',
  color: theme.palette.custom.white,
  backgroundColor: theme.palette.background.buttonPrimaryDefault,
  boxShadow: "none",
  textTransform: "none",
  fontFamily: 'Inter-Medium',
  maxWidth: 'fit-content',
  maxHeight: 'fit-content',
  "&::before": {
    content: '""',
    position: 'absolute',
    inset: -1,
    borderRadius: 20,
    padding: '1px',
    background: `linear-gradient(163.33deg, ${theme.palette.primary.main} 46.22%, ${theme.palette.custom.purpleExtraDark} 161.35%)`,
    WebkitMask: 'linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)',
    WebkitMaskComposite: 'xor',
    maskComposite: 'exclude',
    pointerEvents: 'none'
  },
  "&:hover": {
    backgroundColor: theme.palette.background.buttonPrimaryDefaultHover,
    boxShadow: "none",
    color: theme.palette.primary.main,
  },
  "&.Mui-disabled": {
    color: theme.palette.text.neutralDisabled,
    backgroundColor: theme.palette.background.surfaceNeutralDisabled,
    "&::before": {
      content: 'none',
    },
    border: `1px solid ${theme.palette.background.defaultSubdued}`,
  },
}));

export const PrimaryOutlineButton = styled(Button)(({ theme }) => ({
  position: 'relative',
  borderRadius: 20,
  padding: '8px 16px',
  color: theme.palette.text.defaultSubdued,
  backgroundColor: theme.palette.background.paper,
  boxShadow: "none",
  textTransform: "none",
  fontFamily: 'Inter-Medium',
  "&::before": {
    content: '""',
    position: 'absolute',
    inset: 0,
    borderRadius: 20,
    padding: '1px',
    background: `linear-gradient(163.33deg, ${theme.palette.primary.main} 46.22%, ${theme.palette.custom.purpleExtraDark} 161.35%)`,
    WebkitMask: 'linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)',
    WebkitMaskComposite: 'xor',
    maskComposite: 'exclude',
    pointerEvents: 'none'
  },
  "&:hover": {
    backgroundColor: theme.palette.background.buttonPrimaryOutlineHover,
    boxShadow: "none",
    color: theme.palette.text.defaultSubdued,
  },
}));

export const DangerButton = styled(Button)(({ theme }) => ({
  position: 'relative',
  borderRadius: 20,
  padding: '8px 16px',
  color: theme.palette.custom.white,
  backgroundColor: theme.palette.background.buttonCritical,
  border: `1px solid ${theme.palette.border.criticalDefault}`,
  boxShadow: "none",
  textTransform: "none",
  fontFamily: 'Inter-Medium',
  "&:hover": {
    backgroundColor: theme.palette.background.buttonCriticalHover,
    boxShadow: "none",
    color: theme.palette.custom.white,
    border: `1px solid ${theme.palette.border.criticalHover}`
  },
}));

export const DangerOutlineButton = styled(Button)(({ theme }) => ({
  position: 'relative',
  borderRadius: 20,
  padding: '8px 16px',
  color: theme.palette.background.buttonCritical,
  backgroundColor: theme.palette.custom.white,
  border: `1px solid ${theme.palette.background.buttonCritical}`,
  boxShadow: "none",
  textTransform: "none",
  fontFamily: 'Inter-Medium',
  "&:hover": {
    backgroundColor: theme.palette.border.criticalDefaultSubdue,
    boxShadow: "none",
    color: theme.palette.background.buttonCriticalHover,
    border: `1px solid ${theme.palette.background.buttonCriticalHover}`
  },
}));

export const SecondaryLinkButton = styled(Button)(({ theme }) => ({
  color: theme.palette.text.defaultSubdued,
  backgroundColor: 'transparent',
  cursor: 'pointer',
  fontFamily: 'Inter-Semibold',
  display: 'flex',
  alignItems: 'center',
  border: 'none',
  padding: '0 8px 0 0',
  textTransform: 'none',
  '&:hover': {
    color: theme.palette.text.defaultSubdued,
    backgroundColor: 'transparent',
    border: 'none'
  }
}));

export const SecondaryOutlineButton = styled(Button)(({ theme }) => ({
  position: 'relative',
  borderRadius: 20,
  padding: '8px 16px',
  color: theme.palette.text.defaultSubdued,
  backgroundColor: theme.palette.background.paper,
  border: `1px solid ${theme.palette.border.neutralDefault}`,
  boxShadow: "none",
  textTransform: "none",
  fontFamily: 'Inter-Medium',
  "&:hover": {
    backgroundColor: theme.palette.background.surfaceNeutralHover,
    boxShadow: "none",
    color: theme.palette.text.primary,
    border: `1px solid ${theme.palette.border.neutralHovered}`,
  },
}));

export const StyledDialog = styled(Dialog)(({ theme }) => ({
  "& .MuiDialog-paper": {
    borderRadius: "12px",
    backgroundColor: theme.palette.background.paper,
  },
}));

export const StyledDialogTitle = styled(DialogTitle)(({ theme }) => ({
  backgroundColor: theme.palette.background.default,
  color: theme.palette.text.default,
  padding: theme.spacing(2),
}));

export const StyledDialogContent = styled(DialogContent)(({ theme }) => ({
  padding: theme.spacing(3),
}));

export const StyledNavLink = styled(NavLink)(({ theme }) => ({
  textDecoration: "none",
  color: theme.palette.text.primary,
  "&.active": {
    backgroundColor: theme.palette.custom.teal,
    color: theme.palette.common.black,
    "& .MuiListItemIcon-root": {
      color: theme.palette.common.black,
    },
  },
  "&:hover": {
    backgroundColor: theme.palette.primary.light,
  },
}));

export const StyledIconButton = styled(IconButton)(({ theme }) => ({
  color: theme.palette.text.light,
  "&:hover": {
    backgroundColor: theme.palette.custom.lightTeal,
  },
}));

export const StyledAccordion = styled(Accordion)(({ theme }) => ({
  marginTop: theme.spacing(3),
  boxShadow: "none",
  border: `1px solid ${theme.palette.border.neutralDefault}`,
  borderRadius: `${theme.shape.borderRadius * 3}px`,
  "&:before": {
    display: "none",
  },
  "& .MuiAccordionSummary-root": {
    backgroundColor: theme.palette.background.paper,
    borderRadius: `${theme.shape.borderRadius * 3}px`,
    padding: "16px 24px",
    "&.Mui-expanded": {
      borderBottomLeftRadius: 0,
      borderBottomRightRadius: 0,
    },
  },
  "& .MuiAccordionSummary-content": {
    color: theme.palette.text.primary,
    margin: 0,
  },
  "& .MuiAccordionSummary-expandIconWrapper": {
    color: theme.palette.text.primary,
  },
  "& .MuiAccordionDetails-root": {
    backgroundColor: theme.palette.background.paper,
    borderBottomLeftRadius: `${theme.shape.borderRadius * 3}px`,
    borderBottomRightRadius: `${theme.shape.borderRadius * 3}px`,
    padding: "24px",
  },
  "& .MuiAccordion-root": {
    transition: theme.transitions.create(["margin", "border-radius"]),
  },
}));

export const StyledSelect = styled(Select)(({ theme }) => ({
  height: '32px', 
  backgroundColor: theme.palette.custom.white,
  borderRadius: '8px',
  marginTop: theme.spacing(1),
  '& .MuiOutlinedInput-notchedOutline': {
    borderRadius: '8px',
  },
  '& .MuiSelect-select': {
    height: '32px', 
    display: 'flex',
    alignItems: 'center',
    paddingTop: theme.spacing(1.5),
  },
  '& .MuiSelect-icon': {
    top: '20%',
    right: '7px',
    position: 'absolute',
    transition: 'transform 0.2s', 
  },
  '&.Mui-focused': {
    height: '41px',
    marginTop: 0,
    paddingTop: theme.spacing(2.5),
    '& .MuiSelect-select': {
      paddingTop: 0, 
    },
    '& .MuiSelect-icon': {
      top: '35%',
    },
  },
  '&.MuiOutlinedInput-root.Mui-focused': {
    height: '41px', 
    '& .MuiSelect-select': {
      height: '41px',
    }
  },
  '&&': {
    '& .MuiMenu-paper, & .MuiPopover-paper': {
      borderRadius: '8px',
      marginTop: '4px',
      boxShadow: '0px 5px 15px rgba(0, 0, 0, 0.2)'
    }
  }
}));

export const ResponsiveTitleBox = styled(Box)(({ theme }) => ({
  display: 'flex',
  flexWrap: 'wrap',
  alignItems: 'flex-end',
  justifyContent: 'space-between',
  width: '100%'
}));

export const TitleContentBox = styled(Box)(({ theme }) => ({
  display: 'flex',
  flexDirection: 'column',
  alignItems: 'flex-start',
  [theme.breakpoints.down('sm')]: {
    marginBottom: theme.spacing(2)
  }
}));

export const ActionButtonsBox = styled(Box)(({ theme }) => ({
  display: 'flex',
  gap: theme.spacing(2),
  [theme.breakpoints.down('sm')]: {
    width: '100%',
    justifyContent: 'flex-start'
  }
}));

export const StyledFormControl = styled(FormControl)(({ theme }) => ({
  width: '100%',
  '& .MuiInputLabel-root': {
    transform: 'translate(14px, 9px) scale(1)',
  },
  '& .MuiInputLabel-shrink': {
    transform: 'translate(14px, -9px) scale(0.75)',
  },
}));

export const StyledTextField = styled(TextField)(({ theme }) => ({
  width: '100%',
  height: '36px',
  marginTop: theme.spacing(0.3),
  backgroundColor: theme.palette.custom.white,
  borderRadius: '8px',
  '& .MuiOutlinedInput-root': {
    width: '100%',
    boxSizing: 'border-box',
    borderRadius: '8px',
  },
  '& .MuiInputBase-input': {
    height: '36px',
    width: '100%',
    boxSizing: 'border-box',
  }
}));
