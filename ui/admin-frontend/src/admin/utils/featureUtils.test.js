import { getFeatureFlags } from './featureUtils';

describe('getFeatureFlags', () => {
  test('returns all flags as undefined when no features are provided', () => {
    const result = getFeatureFlags();
    expect(result).toEqual({
      isGatewayOnly: undefined,
      isPortalOnly: undefined,
      isChatOnly: undefined
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
      isPortalOnly: false,
      isChatOnly: false
    });
  });

  test('returns isPortalOnly as true when only portal feature is enabled', () => {
    const features = {
      feature_gateway: false,
      feature_portal: true,
      feature_chat: false
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalOnly: true,
      isChatOnly: false
    });
  });

  test('returns isChatOnly as true when only chat feature is enabled', () => {
    const features = {
      feature_gateway: false,
      feature_portal: false,
      feature_chat: true
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalOnly: false,
      isChatOnly: true
    });
  });

  test('returns all flags as false when multiple features are enabled', () => {
    const features = {
      feature_gateway: true,
      feature_portal: true,
      feature_chat: false
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalOnly: false,
      isChatOnly: false
    });
  });

  test('returns all flags as false when all features are enabled', () => {
    const features = {
      feature_gateway: true,
      feature_portal: true,
      feature_chat: true
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: false,
      isPortalOnly: false,
      isChatOnly: false
    });
  });

  test('returns isGatewayOnly as true when features object has only gateway property', () => {
    const features = {
      feature_gateway: true
    };
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: true,
      isPortalOnly: false,
      isChatOnly: false
    });
  });

  test('safely handles undefined feature properties', () => {
    const features = {};
    const result = getFeatureFlags(features);
    expect(result).toEqual({
      isGatewayOnly: undefined,
      isPortalOnly: undefined,
      isChatOnly: undefined
    });
  });
});