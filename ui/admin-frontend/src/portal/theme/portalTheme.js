import { createTheme } from "@mui/material/styles";
const portalTheme = createTheme({
  palette: {
    primary: {
      main: "#000000",
    },
    background: {
      default: "#ffffff",
      paper: "#ffffff",
    },
    text: {
      primary: "#000000",
      secondary: "#000000",
    },
  },
  components: {
    MuiAppBar: {
      styleOverrides: {
        root: {
          backgroundColor: "#ffffff",
          boxShadow: "none",
          color: "#000000",
        },
      },
    },
    MuiDrawer: {
      styleOverrides: {
        paper: {
          backgroundColor: "#ffffff",
          boxShadow: "none",
          width: "260px",
          padding: "16px",
        },
      },
    },
    MuiListItem: {
      styleOverrides: {
        root: {
          marginTop: "8px",
          borderRadius: "8px",
          "&:hover": {
            backgroundColor: "rgba(0, 0, 0, 0.04)",
          },
        },
      },
    },
    MuiCard: {
      styleOverrides: {
        root: {
          borderRadius: "16px",
          boxShadow: "0px 4px 20px rgba(0, 0, 0, 0.1)",
          backgroundColor: "#ffffff",
        },
      },
    },
  },
  customClasses: {
    topLevelMenu: {
      backgroundColor: "#ffffff",
      borderRadius: "16px",
      boxShadow: "0px 4px 20px rgba(0, 0, 0, 0.1)",
      padding: "16px",
    },
  },
});

export default portalTheme;
