import React from "react";
import { Routes, Route, Navigate } from "react-router-dom";
import { ChatContextProvider } from "../portal/context/ChatContext";
import ChatView from "../portal/components/ChatView";
import ChatDashboard from "../portal/pages/ChatDashboard";

const ChatRoutes = () => (
  <ChatContextProvider>
    <Routes>
      <Route path="/" element={<Navigate to="/chat/dashboard" />} />
      <Route path="/dashboard" element={<ChatDashboard />} />
      <Route path="/:chatId" element={<ChatView />} />
    </Routes>
  </ChatContextProvider>
);

export default ChatRoutes;
