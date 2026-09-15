import React from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import ChatView from "../portal/components/ChatView";
import ChatViewV2 from "../portal/pages/ChatViewV2";
import ChatDashboard from "../portal/pages/ChatDashboard";
import AgentDashboard from "../portal/pages/AgentDashboard";
import AgentChat from "../portal/pages/AgentChat";
import AgentChatV2 from "../portal/pages/AgentChatV2";
import useSystemFeatures from "../admin/hooks/useSystemFeatures";

/**
 * Picks the chat implementation: the assistant-ui based pages when the
 * backend reports chat_ui_v2 (CHAT_UI_V2_ENABLED), otherwise the v1 pages.
 */
const FeatureSwitch = ({ v2, v1 }) => {
  const { features, loading } = useSystemFeatures();
  if (loading) return null;
  return features?.chat_ui_v2 ? v2 : v1;
};

const ChatRoutes = () => (
  <Routes>
    <Route path="/" element={<Navigate to="/chat/dashboard" />} />
    <Route path="/dashboard" element={<ChatDashboard />} />
    <Route path="/agents" element={<AgentDashboard />} />
    <Route path="/agent/:agentId" element={<FeatureSwitch v2={<AgentChatV2 />} v1={<AgentChat />} />} />
    <Route path="/:chatId" element={<FeatureSwitch v2={<ChatViewV2 />} v1={<ChatView />} />} />
  </Routes>
);

export default ChatRoutes;
