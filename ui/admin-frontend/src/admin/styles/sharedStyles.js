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
  Grid,
  DialogTitle,
  Accordion,
  DialogContent,
} from "@mui/material";
import IconButton from "@mui/material/IconButton";
import { NavLink } from "react-router-dom";

export const StyledPaper = styled(Paper)(({ theme }) => ({
  backgroundColor: theme.palette.background.paper,
  borderRadius: theme.shape.borderRadius * 3,
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
}));

export const StyledTableHeaderCell = styled(TableCell)(({ theme }) => ({
  fontWeight: "bold",
}));

export const StyledTableCell = styled(TableCell)(({ theme }) => ({
  textAlign: "left",
}));

export const StyledTableRow = styled(TableRow)(({ theme }) => ({
  "&:nth-of-type(odd)": {
    backgroundColor: theme.palette.custom.lightTeal,
  },
  "&:nth-of-type(even)": {
    backgroundColor: "rgba(255, 255, 255, 0)",
  },
  "&:hover": {
    backgroundColor: theme.palette.custom.hoverTeal,
  },
}));

export const FieldLabel = styled(Typography)(({ theme }) => ({
  fontWeight: "bold",
  color: theme.palette.text.secondary,
}));

export const FieldValue = styled(Typography)(({ theme }) => ({
  color: theme.palette.text.primary,
}));

export const StyledButton = styled(Button)(({ theme }) => ({
  borderRadius: 20,
  border: `1px solid ${theme.palette.custom.purpleExtraDark}`,
  color: theme.palette.custom.white,
  backgroundColor: theme.palette.custom.purpleDark,
  boxShadow: "none",
  textTransform: "Capitalize",
  "&:hover": {
    backgroundColor: theme.palette.custom.purpleExtraDark,
    boxShadow: "none",
  },
}));

export const StyledDialog = styled(Dialog)(({ theme }) => ({
  "& .MuiDialog-paper": {
    borderRadius: "12px",
    backgroundColor: theme.palette.background.paper,
  },
}));

export const StyledDialogTitle = styled(DialogTitle)(({ theme }) => ({
  backgroundColor: theme.palette.custom.purpleLight,
  color: theme.palette.text.light,
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
  "&:before": {
    display: "none",
  },
  "& .MuiAccordionSummary-root": {
    backgroundColor: theme.palette.custom.lightTeal,
    borderRadius: `${theme.shape.borderRadius * 3}px`,
    "&:hover": {
      backgroundColor: theme.palette.custom.teal,
    },
    "&.Mui-expanded": {
      borderBottomLeftRadius: 0,
      borderBottomRightRadius: 0,
    },
  },
  "& .MuiAccordionSummary-content": {
    color: theme.palette.text.primary,
  },
  "& .MuiAccordionSummary-expandIconWrapper": {
    color: theme.palette.text.primary,
  },
  "& .MuiAccordionDetails-root": {
    backgroundColor: theme.palette.background.paper,
    borderBottomLeftRadius: `${theme.shape.borderRadius * 3}px`,
    borderBottomRightRadius: `${theme.shape.borderRadius * 3}px`,
    borderTop: `1px solid ${theme.palette.divider}`,
  },
  "& .MuiAccordion-root": {
    transition: theme.transitions.create(["margin", "border-radius"]),
  },
}));
