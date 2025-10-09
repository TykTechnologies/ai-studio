import React from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import ChatView from "../portal/components/ChatView";
import ChatDashboard from "../portal/pages/ChatDashboard";
import AgentDashboard from "../portal/pages/AgentDashboard";
import AgentChat from "../portal/pages/AgentChat";

const ChatRoutes = () => (
  <Routes>
    <Route path="/" element={<Navigate to="/chat/dashboard" />} />
    <Route path="/dashboard" element={<ChatDashboard />} />
    <Route path="/agents" element={<AgentDashboard />} />
    <Route path="/agent/:agentId" element={<AgentChat />} />
    <Route path="/:chatId" element={<ChatView />} />
  </Routes>
);

export default ChatRoutes;
