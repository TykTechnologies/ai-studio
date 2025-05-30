import { styled } from "@mui/material/styles";
import { Box } from "@mui/material";
import catalogueBadgeConfigs from '../../../pages/groups/utils/catalogueBadgeConfig';

// Helper function to safely access nested palette properties
export const getPaletteColor = (theme, colorPath) => {
  const path = colorPath.split('.');
  let color = theme.palette;
  for (const segment of path) {
    color = color[segment];
    if (!color) break;
  }
  return color;
};

// Export the catalog badge configs for easy access
export { catalogueBadgeConfigs };

// Helper function to get colors for a catalog variant
export const getColorsForVariant = (theme, variant) => {
  if (catalogueBadgeConfigs[variant]) {
    return {
      bgColor: getPaletteColor(theme, catalogueBadgeConfigs[variant].bgColor),
      textColor: getPaletteColor(theme, catalogueBadgeConfigs[variant].textColor)
    };
  }
  
  // No fallback needed - StyledChip handles defaults
  return {};
};

// For catalog display in lists/tables
export const CatalogBadge = styled(Box)(({ theme, variant = 'default' }) => {
  // Get colors from catalogueBadgeConfigs if available
  const getColors = () => {
    if (catalogueBadgeConfigs[variant]) {
      return {
        bgColor: getPaletteColor(theme, catalogueBadgeConfigs[variant].bgColor),
        textColor: getPaletteColor(theme, catalogueBadgeConfigs[variant].textColor)
      };
    }
    
    // Default fallback
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
    fontSize: '0.75rem',
    fontWeight: 500,
  };
});

// Styled component for catalog container to replace inline sx prop
export const CatalogContainer = styled(Box)({
  display: "flex", 
  flexWrap: "wrap", 
  gap: 1, 
  justifyContent: "flex-start"
}); 