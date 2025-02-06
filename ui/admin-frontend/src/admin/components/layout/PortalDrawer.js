import React, { useState, useEffect } from "react";
import {
  Dashboard,
  AddCircleOutline,
  Code,
  Psychology,
  Storage,
  Apps,
} from "@mui/icons-material";
import BaseDrawer from "./BaseDrawer";
import useSystemFeatures from "../../hooks/useSystemFeatures";
import { DRAWER_WIDTH } from "../../../constants/layout";
import pubClient from "../../utils/pubClient";

const CACHE_KEY = "userEntitlements";
const CACHE_EXPIRY = 10000;

const PortalDrawer = () => {
  const { features, loading } = useSystemFeatures();
  const [userEntitlements, setUserEntitlements] = useState(null);
  const [uiOptions, setUiOptions] = useState(null);

  useEffect(() => {
    const fetchUserEntitlements = async () => {
      const cachedData = localStorage.getItem(CACHE_KEY);
      if (cachedData) {
        const { data, timestamp } = JSON.parse(cachedData);
        if (Date.now() - timestamp < CACHE_EXPIRY) {
          setUserEntitlements(data);
          setUiOptions(data.ui_options);
          return;
        }
      }

      try {
        const response = await pubClient.get("/common/me");
        const newData = response.data.attributes.entitlements;
        const newUiOptions = response.data.attributes.ui_options;
        setUserEntitlements(newData);
        setUiOptions(newUiOptions);
        localStorage.setItem(
          CACHE_KEY,
          JSON.stringify({
            data: { ...newData, ui_options: newUiOptions },
            timestamp: Date.now(),
          }),
        );
      } catch (error) {
        console.error("Failed to fetch user entitlements:", error);
      }
    };

    fetchUserEntitlements();
  }, []);

  if (loading) {
    return null;
  }

  const showPortalFeatures = features.feature_portal || features.feature_gateway;

  const getMenuItems = () => {
    if (!showPortalFeatures || !uiOptions?.show_portal) {
      return [];
    }

    return [
      {
        id: "dashboard",
        text: "Dashboard",
        icon: <Dashboard />,
        path: "/portal/dashboard"
      },
      {
        id: "create-app",
        text: "Create App",
        icon: <AddCircleOutline />,
        path: "/portal/app/new"
      },
      {
        id: "resources",
        text: "Resources",
        icon: <Code />,
        subItems: [
          {
            id: "llms",
            text: "LLMs",
            icon: <Psychology />,
            subItems: userEntitlements?.catalogues?.map(catalogue => ({
              id: `llm-${catalogue.id}`,
              text: catalogue.attributes.name,
              path: `/portal/llms/${catalogue.id}`
            }))
          },
          {
            id: "databases",
            text: "Databases",
            icon: <Storage />,
            subItems: userEntitlements?.data_catalogues?.map(catalogue => ({
              id: `db-${catalogue.id}`,
              text: catalogue.attributes.name,
              path: `/portal/databases/${catalogue.id}`
            }))
          }
        ]
      },
      {
        id: "my-apps",
        text: "My Apps",
        icon: <Apps />,
        path: "/portal/apps"
      }
    ];
  };

  return (
    <BaseDrawer
      menuItems={getMenuItems()}
      drawerWidth={DRAWER_WIDTH}
      minimizedWidth={60}
      showToolbar={false}
      customStyles={{
        marginTop: "64px"
      }}
      defaultOpen={true}
    />
  );
};

export default PortalDrawer;
