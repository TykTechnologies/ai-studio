import { styled } from "@mui/material/styles";
import { Box, Typography } from "@mui/material";
import catalogueBadgeConfigs from '../../../pages/groups/utils/catalogueBadgeConfig';

export const getPaletteColor = (theme, colorPath) => {
  const path = colorPath.split('.');
  let color = theme.palette;
  for (const segment of path) {
    color = color[segment];
    if (!color) break;
  }
  return color;
};

export const getColorsForVariant = (theme, variant) => {
  if (catalogueBadgeConfigs[variant]) {
    return {
      bgColor: getPaletteColor(theme, catalogueBadgeConfigs[variant].bgColor),
      textColor: getPaletteColor(theme, catalogueBadgeConfigs[variant].textColor)
    };
  }
  
  return {};
};

export const CatalogBadge = styled(Box)(({ theme, variant = 'default' }) => {
  const getColors = () => {
    if (catalogueBadgeConfigs[variant]) {
      return {
        bgColor: getPaletteColor(theme, catalogueBadgeConfigs[variant].bgColor),
        textColor: getPaletteColor(theme, catalogueBadgeConfigs[variant].textColor)
      };
    }
    
    return {
      bgColor: theme.palette.background.buttonPrimaryOutlineHover,
      textColor: theme.palette.text.defaultSubdued
    };
  };

  const { bgColor, textColor } = getColors();

  return {
    display: 'inline-block',
    backgroundColor: bgColor,
    borderRadius: '6px',
    padding: '2px 8px',
    color: textColor,
  };
});

export const CatalogContainer = styled(Box)(({ theme }) => ({
  display: "flex", 
  flexWrap: "wrap", 
  gap: 5, 
  justifyContent: "flex-start",
  gridColumn: 2,
  alignSelf: "center",
  marginLeft: theme.spacing(6),
}));

export const CatalogsWrapper = styled(Box)(({ theme }) => ({
  display: "grid",
  gridTemplateColumns: "max-content 1fr",
  gap: theme.spacing(2.5),
}));

export const CatalogTypeContainer = styled(Box)({
  display: "contents",
});

export const CatalogBorderLine = styled(Box)(({ theme }) => ({
  gridColumn: "1 / -1",
  borderBottom: "1px solid",
  borderColor: theme.palette.border?.neutralDefaultSubdued,
}));

export const CatalogTypeLabel = styled(Typography)({
  gridColumn: 1,
  alignSelf: "center",
});