import { styled } from "@mui/material/styles";
import { Box, Grid } from "@mui/material";

export const FormSection = styled(Box)(({ theme }) => ({
  marginBottom: theme.spacing(4)
}));

export const FormGrid = styled(Grid)(({ theme }) => ({
  marginBottom: theme.spacing(2)
}));

export const PermissionsContainer = styled(Box)(({ theme }) => ({
  marginTop: theme.spacing(2)
}));

export const ButtonContainer = styled(Box)(({ theme }) => ({
  display: "flex", 
  justifyContent: "flex-start", 
  marginTop: theme.spacing(3), 
  gap: theme.spacing(2)
}));