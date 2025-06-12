import { styled } from "@mui/material/styles";
import { Box, Grid, FormControlLabel } from "@mui/material";
import Icon from "../../../components/common/Icon";
import { getPaletteColor } from "../groups/components/styles";

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

export const RoleOptionBox = styled(FormControlLabel)(({ theme, isLast }) => ({
  padding: theme.spacing(1, 2),
  marginBottom: isLast ? 0 : theme.spacing(2),
  border: '1px solid',
  borderColor: theme.palette.border.neutralDefault,
  borderRadius: '8px',
  '&:hover': {
    backgroundColor: theme.palette.background.surfaceNeutralHover
  }
}));

export const RoleBadge = styled(Box)(({ theme, bgColor }) => ({
  padding: '4px 8px',
  backgroundColor: getPaletteColor(theme, bgColor) || bgColor,
  borderRadius: '6px'
}));

export const PermissionsTooltipBox = styled(Box)(({ theme }) => ({
  border: '1px solid',
  borderColor: theme.palette.border.neutralDefault,
  borderRadius: '8px',
  backgroundColor: theme.palette.background.paper,
  padding: theme.spacing(2),
  height: '100%',
  overflowY: 'auto',
}));

export const StyledPermissionIcon = styled(Icon)(({ theme }) => ({
  width: 16,
  height: 16,
  color: theme.palette.background.iconSuccessDefault,
}));