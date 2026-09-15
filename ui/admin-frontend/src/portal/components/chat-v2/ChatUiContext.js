import React, { createContext, useContext, useMemo, useState } from 'react';

const STORAGE_KEY = 'showSystemMessages';

const ChatUiContext = createContext({
  showSystemMessages: true,
  setShowSystemMessages: () => {},
  session: null,
  userName: '',
});

const readInitial = () => {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    return saved !== null ? JSON.parse(saved) : true;
  } catch (e) {
    return true;
  }
};

/**
 * Per-chat UI settings shared by the message part renderers: whether
 * status/context parts are shown (persisted like the v1 toggle), the current
 * session summary and the display name used for the avatar.
 */
export const ChatUiProvider = ({ session, userName, children }) => {
  const [showSystemMessages, setShow] = useState(readInitial);

  const value = useMemo(
    () => ({
      showSystemMessages,
      setShowSystemMessages: (next) => {
        setShow(next);
        try {
          localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
        } catch (e) {
          // ignore storage failures
        }
      },
      session,
      userName,
    }),
    [showSystemMessages, session, userName],
  );

  return <ChatUiContext.Provider value={value}>{children}</ChatUiContext.Provider>;
};

export const useChatUi = () => useContext(ChatUiContext);

export default ChatUiContext;
