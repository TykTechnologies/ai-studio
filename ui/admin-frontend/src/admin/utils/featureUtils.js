export const getFeatureFlags = (features = {}) => {
  const isGatewayOnly = features.feature_gateway && !features.feature_portal && !features.feature_chat;
  const isPortalOnly = !features.feature_gateway && features.feature_portal && !features.feature_chat;
  const isChatOnly = !features.feature_gateway && !features.feature_portal && features.feature_chat;

  return {
    isGatewayOnly,
    isPortalOnly,
    isChatOnly
  };
}; 