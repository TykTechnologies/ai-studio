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
      main: "#23E2C2",
      light: "#82F5D8", // Light teal for hover
    },
    secondary: {
      main: "#21ecba",
    },
    background: {
      default: "#FFFFFF",
      defaultSubdued:'#E6E6EA', 
      paper: "#FFFFFF",
      surfaceDefaultBoldest: '#031E3D',
      surfaceDefaultHover: '#B7F9E926',
      surfaceDefaultSelected: '#B7F9E973'
    },
    border: {
      neutralDefault: '#D8D8DF'
    },
    gray: {
      ligh: "#F5F5F5",
      main: "#CCCCCC",
      dark: "#333333",
    },
    custom: {
      white: "#FFFFFF",
      leaf: "#21ecba",
      purpleExtraDark: "#5900CB",
      purpleDark: "#8438FA",
      purpleLight: "#B421FA",
      purpleExtraLight: "#F0E4FF",
      teal: "#21ecba",
      lightTeal: "rgb(33 236 186 / 7%)",
      hoverTeal: "rgb(33 236 186 / 47%)",
      emptyStateBackground: "#23ebba11",
    },
    text: {
      light: "#FFFFFF",
      dark: "#023056",
      default: "#03031C",
      defaultSubdued: '#414160'
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
