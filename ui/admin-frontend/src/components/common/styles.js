import { styled } from "@mui/material/styles";
import { AppBar, Toolbar, Tabs, Tab, Box, IconButton } from "@mui/material";

export const StyledAppBar = styled(AppBar)(({ theme }) => ({
  zIndex: theme.zIndex.drawer + 1,
  background: "linear-gradient(91deg, #03031C 42.16%, #23E2C2 72.25%, #5900CB 92.79%, #BB11FF 104.86%)",
}));

export const StyledToolbar = styled(Toolbar)({
  minHeight: "64px",
  padding: "0 24px",
});

export const NavigationContainer = styled(Box)({
  display: "flex",
  flexGrow: 1,
  height: "64px",
});

export const LogoContainer = styled(Box)({
  display: "flex",
  alignItems: "center",
  left: "24px",
});

export const Logo = styled('img')({
  height: "25px",
});

export const TabsContainer = styled(Box)({
  display: "flex",
  alignItems: "flex-end",
  marginLeft: "87px",
  height: "100%",
  paddingBottom: 0,
});

export const StyledTabs = styled(Tabs)({
  gap: "8px",
  minHeight: "unset",
  "& .MuiTabs-flexContainer": {
    gap: "8px",
  },
});

export const StyledTab = styled(Tab)(({ theme }) => ({
  color: "#B7F9E9",
  fontFamily: "Inter-Regular",
  fontSize: "13.2px",
  lineHeight: "20px",
  textTransform: "none",
  height: "28px",
  padding: "2px 20px 6px 12px",
  minHeight: "unset",
  minWidth: "unset",
  alignItems: "flex-end",
  whiteSpace: "nowrap",
  opacity: 1,
  "&.Mui-selected": {
    color: "#23E2C2",
    fontFamily: "Inter-Semibold",
    opacity: 1,
  },
  "&:hover": {
    color: "#23E2C2",
    fontFamily: "Inter-Semibold",
    opacity: 1,
  },
}));

export const StyledLogoutButton = styled(IconButton)({
  color: "white",
});

export const TabIndicatorProps = {
  style: {
    backgroundColor: "#23E2C2",
    height: "1.6px",
  },
};
