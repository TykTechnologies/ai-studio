import { styled } from "@mui/material/styles";
import { Paper, Box, Typography, TableCell, TableRow } from "@mui/material";

export const StyledPaper = styled(Paper)(({ theme }) => ({
  backgroundColor: "#2c2c2c",
  borderRadius: theme.shape.borderRadius * 2,
  overflow: "hidden",
}));

export const TitleBox = styled(Box)(({ theme }) => ({
  backgroundColor: "#0B4545",
  padding: theme.spacing(2),
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
}));

export const ContentBox = styled(Box)(({ theme }) => ({
  padding: theme.spacing(3),
}));

export const StyledTableCell = styled(TableCell)(({ theme }) => ({
  fontWeight: "bold",
}));

export const StyledTableRow = styled(TableRow)(({ theme }) => ({
  "&:nth-of-type(odd)": {
    backgroundColor: "rgba(255, 255, 255, 0.1)",
  },
  "&:nth-of-type(even)": {
    backgroundColor: "rgba(255, 255, 255, 0.15)",
  },
  "&:hover": {
    backgroundColor: "rgba(255, 255, 255, 0.2)",
  },
}));

export const FieldLabel = styled(Typography)(({ theme }) => ({
  fontWeight: "bold",
  color: theme.palette.text.secondary,
}));

export const FieldValue = styled(Typography)(({ theme }) => ({
  color: theme.palette.text.primary,
}));
