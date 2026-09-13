import React, { useState, useEffect } from "react";
import { Outlet, useLocation } from "react-router-dom";
import { ThemeProvider } from "@mui/material/styles";
import { Box, Container } from "@mui/material";
import TopNavigation from "../components/common/TopNavigation";
import AdminLayout from "../admin/components/layout/MainLayout";
import ChatDrawer from "../admin/components/layout/ChatDrawer";
import PortalDrawer from "../admin/components/layout/PortalDrawer";
import { useNavigate } from "react-router-dom";
import { logout } from "../admin/utils/pubClient";
import adminTheme from "../admin/theme";
import { DRAWER_WIDTH, CONTENT_MAX_WIDTH } from "../constants/layout";
import useSystemFeatures from "../admin/hooks/useSystemFeatures";
import { usePermissions } from "../admin/context/PermissionsContext";

const MainLayout = () => {
  const { features } = useSystemFeatures();
  const { identity, isFullAdmin, hasAdminAccess } = usePermissions();
  const [currentTab, setCurrentTab] = useState(null);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();
  const location = useLocation();
  // Identity comes from the single /common/me fetch in App.js.
  const entitlements = identity?.raw?.attributes || null;

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

  // Navigate to the remembered drawer selection for a tab, but only when the
  // current URL is that tab's root (with or without a trailing slash).
  const restoreStoredPath = (tab, root) => {
    const path = location.pathname.replace(/\/+$/, '') || '/';
    if (path !== root) {
      return;
    }
    const storedPath = getStoredPath(tab);
    if (storedPath && storedPath !== location.pathname) {
      navigate(storedPath, { replace: true });
    }
  };

  useEffect(() => {
    const initialiseTab = () => {
      if (location.pathname === '/login') {
        setLoading(false);
        return;
      }

      try {
        // If we're a full admin and either at root or portal dashboard,
        // force redirect to admin dashboard
        if (
          isFullAdmin &&
          (location.pathname === "/" ||
            location.pathname === "/portal/dashboard")
        ) {
          const storedAdminPath = getStoredPath('admin');
          setCurrentTab("admin");
          navigate(storedAdminPath || "/admin", { replace: true });
        } else {
          // Set initial tab based on current location. The stored drawer
          // selection is only restored when the user lands on the bare tab
          // root; a deeper URL (a bookmark, a shared link, a page refresh on
          // /admin/llms/new) is an explicit destination and must win.
          if (location.pathname.startsWith("/admin")) {
            setCurrentTab("admin");
            restoreStoredPath('admin', '/admin');
          } else if (location.pathname.startsWith("/chat")) {
            setCurrentTab("chat");
            restoreStoredPath('chat', '/chat');
          } else if (location.pathname.startsWith("/portal")) {
            setCurrentTab("portal");
            restoreStoredPath('portal', '/portal');
          }
        }

        setLoading(false);
      } catch (error) {
        console.error("Error initialising layout:", error);
        setLoading(false);
      }
    };

    initialiseTab();
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
      default:
        break;
    }
  };

  if (loading) return null;

  const showAdmin = hasAdminAccess;
  const showChat = entitlements?.ui_options?.show_chat && features.feature_chat;
  const showPortal =
    entitlements?.ui_options?.show_portal && features.feature_portal;

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
            <ChatDrawer chats={entitlements?.chats} open />
          )}
          {currentTab === "portal" && showPortal && (
            <PortalDrawer
              catalogues={entitlements?.catalogues}
              dataCatalogues={entitlements?.data_catalogues}
              toolCatalogues={entitlements?.tool_catalogues}
              open
            />
          )}
          <Box
            component="main"
            sx={{
              flexGrow: 1,
              marginTop: "64px",
              width: { sm: `calc(100% - ${DRAWER_WIDTH}px)` },
              minWidth: 0,
            }}
          >
            {currentTab === "chat" ? (
              // Chat views stay full-width
              <Outlet />
            ) : (
              // Portal and common views share the same constrained, centred width as the admin UI
              <Container maxWidth={CONTENT_MAX_WIDTH} disableGutters>
                <Outlet />
              </Container>
            )}
          </Box>
        </Box>
      )}
    </ThemeProvider>
  );
};

export default MainLayout;
