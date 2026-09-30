import React, { useMemo } from 'react';
import BaseDrawer from './base-drawer';
import useNavManifest, { toMenuItems } from '../../hooks/useNavManifest';

/**
 * Chat navigation, from the navigation manifest (GET /common/nav, api/nav.go
 * chatNav): the chat rooms the user may use, their most recent
 * conversations and the agents they may talk to.
 */
const ChatDrawer = () => {
  const { manifest, loading } = useNavManifest();
  const menuItems = useMemo(() => toMenuItems(manifest?.chat || []), [manifest]);

  if (loading) {
    return null;
  }

  return (
    <BaseDrawer
      id="chat"
      menuItems={menuItems}
      showToolbar={false}
      customStyles={{
        marginTop: 'var(--studio-header-height)'
      }}
      defaultExpandedItems={{
        'chat-rooms': true,
        'agents': true
      }}
    />
  );
};

export default ChatDrawer;
