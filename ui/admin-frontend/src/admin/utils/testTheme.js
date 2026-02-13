import { createTheme } from "@mui/material/styles";

const testTheme = createTheme({
  palette: {
    background: {
      paper: "#ffffff",
      buttonPrimaryDefault: "#000000",
      buttonPrimaryDefaultHover: "#333333",
      buttonPrimaryOutlineHover: "#f5f5f5",
      neutralDefault: "#f5f5f5",
      secondaryExtraLight: "#fafafa",
      surfaceNeutralDisabled: "#eeeeee",
    },
    border: {
      neutralDefault: "#cccccc",
    },
    text: {
      primary: "#000000",
      defaultSubdued: "#666666",
      neutralDisabled: "#999999",
    },
    custom: {
      white: "#ffffff",
      purpleExtraDark: "#5900CB",
    },
  },
  spacing: (factor) => `${0.25 * factor}rem`,
});

export default testTheme;
