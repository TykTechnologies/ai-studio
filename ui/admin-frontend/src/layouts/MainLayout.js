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
import useSystemFeatures from "../admin/hooks/useSystemFeatures";

const MainLayout = () => {
  const { features } = useSystemFeatures();
  const [currentTab, setCurrentTab] = useState(null);
  const [entitlements, setEntitlements] = useState(null);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();
  const location = useLocation();

  console.log("Features:", features);
  console.log("Entitlements:", entitlements);

  useEffect(() => {
    const fetchEntitlements = async () => {
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
          setCurrentTab("admin");
          navigate("/admin/dash", { replace: true }); // Added replace: true
        } else {
          // Set initial tab based on current location
          if (location.pathname.startsWith("/admin")) {
            setCurrentTab("admin");
          } else if (location.pathname.startsWith("/chat")) {
            setCurrentTab("chat");
          } else if (location.pathname.startsWith("/portal")) {
            setCurrentTab("portal");
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
    console.log("Tab change requested:", tab);
    setCurrentTab(tab);

    switch (tab) {
      case "chat":
        navigate("/chat/dashboard");
        break;
      case "portal":
        navigate("/portal/dashboard");
        break;
      case "admin":
        navigate("/admin/dash");
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
      onLogout={handleLogout}
    />
  );

  // If we're in admin section, use the admin layout with TopNavigation
  if (currentTab === "admin") {
    return (
      <ThemeProvider theme={adminTheme}>
        <Box sx={{ display: "flex", flexDirection: "column" }}>
          {topNav}
          <Box sx={{ mt: "64px" }}>
            {" "}
            {/* Add margin-top to account for TopNavigation */}
            <AdminLayout hideAppBar />
          </Box>
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
