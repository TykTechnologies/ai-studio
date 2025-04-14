import React, { useState, useEffect } from "react";
import { Outlet, useLocation } from "react-router-dom";
import { ThemeProvider } from "@mui/material/styles";
import { Box } from "@mui/material";
import TopNavigation from "../components/common/TopNavigation";
import AdminLayout from "../admin/components/layout/MainLayout";
import ChatDrawer from "../admin/components/layout/ChatDrawer";
import PortalDrawer from "../admin/components/layout/PortalDrawer";
import { useNavigate } from "react-router-dom";
import pubClient, { logout } from "../admin/utils/pubClient";
import adminTheme from "../admin/theme";
import { DRAWER_WIDTH } from "../constants/layout";
import useSystemFeatures from "../admin/hooks/useSystemFeatures";

const MainLayout = () => {
  const { features } = useSystemFeatures();
  const [currentTab, setCurrentTab] = useState(null);
  const [entitlements, setEntitlements] = useState(null);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();
  const location = useLocation();

  const getStoredPath = (tab) => {
    try {
      const key = `drawer_state_${tab}`;
      const state = localStorage.getItem(key);
      if (state) {
        const { selectedPath } = JSON.parse(state);
        return selectedPath;
      }
    } catch (error) {
      console.error('Error reading stored path:', error);
    }
    return null;
  };

  useEffect(() => {
    const fetchEntitlements = async () => {
      if (location.pathname === '/login') {
        setLoading(false);
        return;
      }

      try {
        const response = await pubClient.get("/common/me");
        const attributes = response.data.attributes;
        setEntitlements(attributes);

        // If we're an admin user and either at root or portal dashboard,
        // force redirect to admin dashboard
        if (
          attributes.is_admin &&
          (location.pathname === "/" ||
            location.pathname === "/portal/dashboard")
        ) {
          const storedAdminPath = getStoredPath('admin');
          setCurrentTab("admin");
          navigate(storedAdminPath || "/admin", { replace: true });
        } else {
          // Set initial tab based on current location
          if (location.pathname.startsWith("/admin")) {
            const storedPath = getStoredPath('admin');
            setCurrentTab("admin");
            if (storedPath && storedPath !== location.pathname) {
              navigate(storedPath, { replace: true });
            }
          } else if (location.pathname.startsWith("/chat")) {
            const storedPath = getStoredPath('chat');
            setCurrentTab("chat");
            if (storedPath && storedPath !== location.pathname) {
              navigate(storedPath, { replace: true });
            }
          } else if (location.pathname.startsWith("/portal")) {
            const storedPath = getStoredPath('portal');
            setCurrentTab("portal");
            if (storedPath && storedPath !== location.pathname) {
              navigate(storedPath, { replace: true });
            }
          }
        }

        setLoading(false);
      } catch (error) {
        console.error("Error fetching entitlements:", error);
        setLoading(false);
      }
    };

    fetchEntitlements();
  }, []); // Only run on mount

  // Second useEffect to handle path changes
  useEffect(() => {
    // Don't update if we're still loading
    if (!loading && location.pathname !== "/") {
      const newTab = location.pathname.startsWith("/admin")
        ? "admin"
        : location.pathname.startsWith("/chat")
          ? "chat"
          : location.pathname.startsWith("/portal")
            ? "portal"
            : null;

      if (newTab) {
        setCurrentTab(newTab);
      }
    }
  }, [location.pathname, loading]);

  const handleTabChange = (tab) => {
    setCurrentTab(tab);

    const storedPath = getStoredPath(tab);

    switch (tab) {
      case "chat":
        navigate(storedPath || "/chat/dashboard");
        break;
      case "portal":
        navigate(storedPath || "/portal/dashboard");
        break;
      case "admin":
        navigate(storedPath || "/admin");
        break;
    }
  };

  if (loading) return null;

  const showAdmin = entitlements?.is_admin;
  const showChat = entitlements?.ui_options?.show_chat && features.feature_chat;
  const showPortal =
    entitlements?.ui_options?.show_portal && features.feature_portal;

  console.log("Show flags:", { showAdmin, showChat, showPortal });

  const topNav = (
    <TopNavigation
      showAdmin={showAdmin}
      showChat={showChat}
      showPortal={showPortal}
      currentTab={currentTab}
      onTabChange={handleTabChange}
      onLogout={logout}
    />
  );

  return (
    <ThemeProvider theme={adminTheme}>
      {currentTab === "admin" ? (
        <Box sx={{ display: "flex", flexDirection: "column" }}>
          {topNav}
          <Box sx={{ mt: "64px" }}>
            <AdminLayout hideAppBar />
          </Box>
        </Box>
      ) : (
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
              marginTop: "64px",
              width: { sm: `calc(100% - ${DRAWER_WIDTH}px)` },
            }}
          >
            <Outlet />
          </Box>
        </Box>
      )}
    </ThemeProvider>
  );
};

export default MainLayout;
