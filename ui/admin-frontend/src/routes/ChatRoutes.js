import React from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import ChatView from "../portal/components/ChatView";
import ChatDashboard from "../portal/pages/ChatDashboard";

const ChatRoutes = () => (
  <Routes>
    <Route path="/" element={<Navigate to="/chat/dashboard" />} />
    <Route path="/dashboard" element={<ChatDashboard />} />
    <Route path="/:chatId" element={<ChatView />} />
  </Routes>
);

export default ChatRoutes;
