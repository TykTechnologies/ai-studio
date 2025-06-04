export const getFeatureFlags = (features = {}) => {
  const hasGateway = !!features.feature_gateway;
  const hasPortal = !!features.feature_portal;
  const hasChat = !!features.feature_chat;
  
  const isGatewayOnly = hasGateway && !hasPortal && !hasChat;
  
  return {
    isGatewayOnly,
    isPortalEnabled: hasPortal,
    isChatEnabled: hasChat,
  };
};