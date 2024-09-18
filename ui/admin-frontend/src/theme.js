import { createTheme } from "@mui/material/styles";

const theme = createTheme({
  typography: {
    fontFamily: ["Inter", "sans-serif"].join(","),
    fontOpticalSizing: "auto",
    fontStyle: "normal",
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
    custom: {
      leaf: "#21ecba",
      purpleDark: "#8437fa",
      purpleLight: "#972afc",
      teal: "#21ecba",
      lightTeal: "rgb(33 236 186 / 7%)",
      hoverTeal: "rgb(33 236 186 / 47%)",
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
          color: "#FFFFFF", // or use a theme color like theme.palette.common.white
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
  },
});

export default theme;
