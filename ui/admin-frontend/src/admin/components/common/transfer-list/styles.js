import { Box, Paper, IconButton, TableRow } from "@mui/material";
import { styled } from "@mui/material/styles";

export const BOX_MAX_HEIGHT = "400px";

export const TransferListContainer = styled(Box)(({ theme }) => ({
  display: "flex",
  gap: theme.spacing(3),
  width: "100%",
  [theme.breakpoints.down("md")]: {
    flexDirection: "column",
  },
}));

export const TransferBox = styled(Paper)(({ theme }) => ({
  border: `1px solid ${theme.palette.border.neutralDefault}`,
  borderRadius: "8px",
  flex: 1,
  padding: theme.spacing(2),
  overflowY: "auto",
  overflowX: "hidden",
  display: "flex",
  flexDirection: "column",
  boxShadow: "none",
  height: BOX_MAX_HEIGHT,
  width: "100%",
}));

export const HeaderBox = styled(Box)(({ theme }) => ({
  display: "flex",
  flexDirection: "column",
  marginBottom: theme.spacing(1.5),
}));

export const SearchContainer = styled(Box)(({ theme }) => ({
  marginBottom: theme.spacing(2),
}));

export const AddButton = styled(IconButton)(({ theme }) => ({
  border: `1.2px solid ${theme.palette.border.neutralDefault}`,
  borderRadius: "50%",
  padding: "4px",
  "& .MuiSvgIcon-root": {
    color: theme.palette.border.neutralPressed,
    fontSize: "16px",
  },
}));

export const RemoveButton = styled(IconButton)(({ theme }) => ({
  border: `1.2px solid ${theme.palette.background.buttonCritical}`,
  borderRadius: "50%",
  padding: "4px",
  "& .MuiSvgIcon-root": {
    color: theme.palette.background.buttonCritical,
    fontSize: "16px",
  },
}));

export const TableHeaderRow = styled(TableRow)(({ theme }) => ({
  backgroundColor: `${theme.palette.background.surfaceNeutralDisabled} !important`,
}));