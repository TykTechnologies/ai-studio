import { createTheme } from "@mui/material/styles";

const theme = createTheme({
  typography: {
    fontFamily: ["Inter-Regular", "sans-serif"].join(","),
    fontOpticalSizing: "auto",
    fontStyle: "normal",
    body1: {
      fontSize: "0.89rem",
    },
    body2: {
      fontSize: "0.89rem",
    },
  },
  palette: {
    mode: "light",
    primary: {
      main: "#CCCCCC",
      light: "#21ecba1a", // Light teal for hover
    },
    secondary: {
      main: "#21ecba",
    },
    background: {
      default: "#FFFFFF",
      paper: "#FFFFFF",
    },
    gray: {
      ligh: "#F5F5F5",
      main: "#CCCCCC",
      dark: "#333333",
    },
    custom: {
      white: "#FFFFFF",
      leaf: "#21ecba",
      purpleExtraDark: "#5525AA",
      purpleDark: "#8437fa",
      purpleLight: "#972afc",
      purpleExtraLight: "#F0E4FF",
      teal: "#21ecba",
      lightTeal: "rgb(33 236 186 / 7%)",
      hoverTeal: "rgb(33 236 186 / 47%)",
      emptyStateBackground: "#23ebba11",
    },
    text: {
      light: "#FFFFFF",
      dark: "#000000",
      default: "#000000",
    },
  },
  components: {
    MuiButton: {
      styleOverrides: {
        root: {
          color: "black",
        },
      },
      variants: [
        {
          props: { color: "white" },
          style: {
            color: "white",
            "&:hover": {
              backgroundColor: "rgba(255, 255, 255, 0.08)",
            },
          },
        },
      ],
    },
    MuiTypography: {
      styleOverrides: {
        h5: {
          fontWeight: "bold",
        },
      },
    },
    MuiTableRow: {
      styleOverrides: {
        root: {
          "&:nth-of-type(odd)": {
            backgroundColor: "rgba(255, 255, 255, 1)",
          },
        },
      },
    },
    MuiTableCell: {
      styleOverrides: {
        root: {
          borderBottom: "1px solid rgba(255, 255, 255, 0.98)",
        },
      },
    },
    MuiPaper: {
      variants: [
        {
          props: { variant: "emptyState" },
          style: {
            backgroundColor: "#23ebba11", // Light blue color
            border: "1px solid #90CAF9", // Slightly darker blue for border
          },
        },
      ],
    },
    MuiInputLabel: {
      styleOverrides: {
        shrink: {
          transform: "translate(14px, -9px) scale(0.75)", // Adjust these values as needed
        },
      },
    },
    MuiFormLabel: {
      styleOverrides: {
        root: {
          "&.Mui-focused, &.MuiFormLabel-filled": {
            transform: "translate(2px, -16px) scale(0.75)", // Match the InputLabel
          },
        },
      },
    },
  },
});

export default theme;
