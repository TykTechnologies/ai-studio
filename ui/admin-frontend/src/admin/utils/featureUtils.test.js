import { getFeatureFlags } from './featureUtils';

describe('getFeatureFlags', () => {
  test('returns all flags as false when no features are provided', () => {
    const result = getFeatureFlags();
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalEnabled: false,
      isChatEnabled: false
    });
  });

  test('returns isGatewayOnly as true when only gateway feature is enabled', () => {
    const features = {
      feature_gateway: true,
      feature_portal: false,
      feature_chat: false
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: true,
      isPortalEnabled: false,
      isChatEnabled: false
    });
  });

  test('returns isPortalEnabled as true when portal feature is enabled', () => {
    const features = {
      feature_gateway: false,
      feature_portal: true,
      feature_chat: false
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalEnabled: true,
      isChatEnabled: false
    });
  });

  test('returns isChatEnabled as true when chat feature is enabled', () => {
    const features = {
      feature_gateway: false,
      feature_portal: false,
      feature_chat: true
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalEnabled: false,
      isChatEnabled: true
    });
  });

  test('returns correct flags when gateway and portal features are enabled', () => {
    const features = {
      feature_gateway: true,
      feature_portal: true,
      feature_chat: false
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalEnabled: true,
      isChatEnabled: false
    });
  });

  test('returns correct flags when all features are enabled', () => {
    const features = {
      feature_gateway: true,
      feature_portal: true,
      feature_chat: true
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalEnabled: true,
      isChatEnabled: true
    });
  });

  test('returns isGatewayOnly as true when features object has only gateway property', () => {
    const features = {
      feature_gateway: true
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: true,
      isPortalEnabled: false,
      isChatEnabled: false
    });
  });

  test('safely handles undefined feature properties', () => {
    const features = {};
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalEnabled: false,
      isChatEnabled: false
    });
  });

  test('returns correct flags when gateway and chat features are enabled', () => {
    const features = {
      feature_gateway: true,
      feature_portal: false,
      feature_chat: true
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalEnabled: false,
      isChatEnabled: true
    });
  });

  test('returns correct flags when portal and chat features are enabled', () => {
    const features = {
      feature_gateway: false,
      feature_portal: true,
      feature_chat: true
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalEnabled: true,
      isChatEnabled: true
    });
  });

  test('returns isPortalEnabled as true when only portal property exists and is true', () => {
    const features = {
      feature_portal: true
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalEnabled: true,
      isChatEnabled: false
    });
  });

  test('returns isChatEnabled as true when only chat property exists and is true', () => {
    const features = {
      feature_chat: true
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalEnabled: false,
      isChatEnabled: true
    });
  });
});