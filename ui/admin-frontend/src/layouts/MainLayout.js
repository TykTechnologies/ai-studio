import React, { useState, useEffect } from "react";
import { Outlet, useLocation } from "react-router-dom";
import { ThemeProvider } from "@mui/material/styles";
import { Box } from "@mui/material";
import TopNavigation from "../components/common/TopNavigation";
import AdminLayout from "../admin/components/layout/MainLayout";
import ChatDrawer from "../admin/components/layout/ChatDrawer";
import PortalDrawer from "../admin/components/layout/PortalDrawer";
import { useNavigate } from "react-router-dom";
import pubClient from "../admin/utils/pubClient";
import adminTheme from "../admin/theme";
import portalTheme from "../portal/theme/portalTheme";
import { DRAWER_WIDTH } from "../constants/layout";

const MainLayout = () => {
  const [currentTab, setCurrentTab] = useState("chat");
  const [entitlements, setEntitlements] = useState(null);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();
  const location = useLocation();

  useEffect(() => {
    const fetchEntitlements = async () => {
      try {
        const response = await pubClient.get("/common/me");
        setEntitlements(response.data.attributes);
        setLoading(false);
      } catch (error) {
        console.error("Error fetching entitlements:", error);
        setLoading(false);
      }
    };

    fetchEntitlements();
  }, []);

  useEffect(() => {
    const path = location.pathname;
    if (path.startsWith("/admin")) {
      setCurrentTab("admin");
    } else if (path.startsWith("/chat")) {
      setCurrentTab("chat");
    } else if (path.startsWith("/portal")) {
      setCurrentTab("portal");
    }
  }, [location]);

  const handleTabChange = (tab) => {
    setCurrentTab(tab);
    switch (tab) {
      case "chat":
        navigate("/chat/dashboard");
        break;
      case "portal":
        navigate("/portal/dashboard");
        break;
      case "admin":
        navigate("/admin/dashboard");
        break;
    }
  };

  const handleLogout = async () => {
    try {
      await pubClient.post("/common/logout");
      navigate("/login");
    } catch (error) {
      console.error("Logout failed:", error);
    }
  };

  if (loading) return null;

  const showAdmin = entitlements?.is_admin;
  const showChat = entitlements?.ui_options?.show_chat;
  const showPortal = entitlements?.ui_options?.show_portal;

  const topNav = (
    <TopNavigation
      showAdmin={showAdmin}
      showChat={showChat}
      showPortal={showPortal}
      currentTab={currentTab}
      onTabChange={handleTabChange}
      onLogout={handleLogout}
    />
  );

  // If we're in admin section, use the admin layout with TopNavigation
  if (currentTab === "admin") {
    return (
      <ThemeProvider theme={adminTheme}>
        <Box sx={{ display: "flex", flexDirection: "column" }}>
          {topNav}
          <AdminLayout hideAppBar />
        </Box>
      </ThemeProvider>
    );
  }

  // Otherwise use the portal/chat layout
  return (
    <ThemeProvider theme={portalTheme}>
      <Box sx={{ display: "flex" }}>
        {topNav}

        {currentTab === "chat" && showChat && (
          <ChatDrawer chats={entitlements.chats} open />
        )}
        {currentTab === "portal" && showPortal && (
          <PortalDrawer
            catalogues={entitlements.catalogues}
            dataCatalogues={entitlements.data_catalogues}
            open
          />
        )}

        <Box
          component="main"
          sx={{
            flexGrow: 1,
            p: 3,
            marginTop: "64px",
            width: { sm: `calc(100% - ${DRAWER_WIDTH}px)` },
            ml: "20px",
          }}
        >
          <Outlet />
        </Box>
      </Box>
    </ThemeProvider>
  );
};

export default MainLayout;
