import React, { useState, useEffect } from "react";
import {
  Chat,
  Dashboard,
  ChatBubbleOutline,
} from "@mui/icons-material";
import BaseDrawer from "./BaseDrawer";
import { DRAWER_WIDTH } from "../../../constants/layout";
import pubClient from "../../utils/pubClient";
import useSystemFeatures from "../../hooks/useSystemFeatures";

const CACHE_KEY = "userEntitlements";
const CACHE_EXPIRY = 10000;

const ChatDrawer = () => {
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

  const getMenuItems = () => {
    if (!features.feature_chat || !uiOptions?.show_chat) {
      return [];
    }

    return [
      {
        id: "dashboard",
        text: "Dashboard",
        icon: <Dashboard />,
        path: "/chat/dashboard"
      },
      {
        id: "chat-rooms",
        text: "Chat Rooms",
        icon: <Chat />,
        subItems: userEntitlements?.chats?.map((chat) => ({
          id: `chat-${chat.id}`,
          text: chat.attributes.name,
          icon: <ChatBubbleOutline />,
          path: `/chat/${chat.id}`
        }))
      }
    ];
  };

  return (
    <BaseDrawer
      menuItems={getMenuItems()}
      showToolbar={false}
      customStyles={{
        marginTop: "64px"
      }}
      defaultExpandedItems={{
        "chat-rooms": true
      }}
    />
  );
};

export default ChatDrawer;
