import React from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import ChatView from "../portal/components/ChatView";
import ChatViewV2 from "../portal/pages/ChatViewV2";
import ChatDashboard from "../portal/pages/ChatDashboard";
import AgentDashboard from "../portal/pages/AgentDashboard";
import AgentChat from "../portal/pages/AgentChat";
import useSystemFeatures from "../admin/hooks/useSystemFeatures";

/**
 * Picks the chat room implementation: the assistant-ui based page when the
 * backend reports chat_ui_v2 (CHAT_UI_V2_ENABLED), otherwise the v1 page.
 */
const ChatRoomRoute = () => {
  const { features, loading } = useSystemFeatures();
  if (loading) return null;
  return features?.chat_ui_v2 ? <ChatViewV2 /> : <ChatView />;
};

const ChatRoutes = () => (
  <Routes>
    <Route path="/" element={<Navigate to="/chat/dashboard" />} />
    <Route path="/dashboard" element={<ChatDashboard />} />
    <Route path="/agents" element={<AgentDashboard />} />
    <Route path="/agent/:agentId" element={<AgentChat />} />
    <Route path="/:chatId" element={<ChatRoomRoute />} />
  </Routes>
);

export default ChatRoutes;
