import React, { createContext, useContext, useState } from 'react';

const ChatContext = createContext();

export const ChatContextProvider = ({ children }) => {
  const [pendingMessage, setPendingMessage] = useState(null);

  const setPendingChatMessage = (chatId, message, files = []) => {
    setPendingMessage({
      chatId,
      message,
      files,
    });
  };

  const clearPendingMessage = () => {
    setPendingMessage(null);
  };

  return (
    <ChatContext.Provider
      value={{
        pendingMessage,
        setPendingChatMessage,
        clearPendingMessage,
      }}
    >
      {children}
    </ChatContext.Provider>
  );
};

export const useChatContext = () => {
  const context = useContext(ChatContext);
  if (!context) {
    throw new Error('useChatContext must be used within a ChatContextProvider');
  }
  return context;
};
