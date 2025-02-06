import { createTheme } from "@mui/material/styles";
const portalTheme = createTheme({
  typography: {
    // This will reduce the base font size (default is typically 14px)
    fontSize: 13, // or any other value you prefer
  },
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
    MuiTypography: {
      styleOverrides: {
        h5: {
          color: "#000000 !important", // This will make all h5 Typography components black
        },
      },
    },
    MuiAppBar: {
      styleOverrides: {
        root: {
          backgroundColor: "#E0F7F6", // Light turquoise color
          boxShadow: "0px 5px 8px rgba(0, 0, 0, 0.2)", // Add this drop shadow
          color: "#000000",
        },
      },
    },
    MuiDrawer: {
      styleOverrides: {
        paper: {
          backgroundColor: "#000000",
          boxShadow: "none",
          width: "260px",
          padding: "16px",
          color: "#ffffff",
          "& .MuiListItemIcon-root": {
            color: "#ffffff",
          },
          "& .MuiListItemText-root": {
            color: "#ffffff",
          },
          "& .MuiIconButton-root": {
            color: "#ffffff",
          },
        },
      },
    },
    MuiListItem: {
      styleOverrides: {
        root: {
          marginTop: "8px",
          borderRadius: "8px",
          "&:hover": {
            backgroundColor: "rgba(255, 255, 255, 0.1)",
          },
          "&.active": {
            backgroundColor: "rgba(255, 255, 255, 0.15)",
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
