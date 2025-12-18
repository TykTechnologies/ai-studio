import React from 'react';
import { renderHook } from '@testing-library/react';
import useFeatureGate from './useFeatureGate';
import { useEdition } from '../context/EditionContext';

// Mock the EditionContext
jest.mock('../context/EditionContext');

describe('useFeatureGate', () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  describe('Enterprise edition', () => {
    beforeEach(() => {
      useEdition.mockReturnValue({ isEnterprise: true });
    });

    test('should return true for enterprise-only features', () => {
      const { result } = renderHook(() => useFeatureGate('budgetEnforcement'));
      expect(result.current).toBe(true);
    });

    test('should return true for budgetAlerts', () => {
      const { result } = renderHook(() => useFeatureGate('budgetAlerts'));
      expect(result.current).toBe(true);
    });

    test('should return true for ssoIntegration', () => {
      const { result } = renderHook(() => useFeatureGate('ssoIntegration'));
      expect(result.current).toBe(true);
    });

    test('should return true for advancedAnalytics', () => {
      const { result } = renderHook(() => useFeatureGate('advancedAnalytics'));
      expect(result.current).toBe(true);
    });

    test('should return true for auditLogs', () => {
      const { result } = renderHook(() => useFeatureGate('auditLogs'));
      expect(result.current).toBe(true);
    });

    test('should return true for prioritySupport', () => {
      const { result } = renderHook(() => useFeatureGate('prioritySupport'));
      expect(result.current).toBe(true);
    });

    test('should return true for community features', () => {
      const { result: basicAnalytics } = renderHook(() => useFeatureGate('basicAnalytics'));
      const { result: costTracking } = renderHook(() => useFeatureGate('costTracking'));
      const { result: proxyGateway } = renderHook(() => useFeatureGate('proxyGateway'));

      expect(basicAnalytics.current).toBe(true);
      expect(costTracking.current).toBe(true);
      expect(proxyGateway.current).toBe(true);
    });
  });

  describe('Community edition', () => {
    beforeEach(() => {
      useEdition.mockReturnValue({ isEnterprise: false });
    });

    test('should return false for enterprise-only features', () => {
      const { result } = renderHook(() => useFeatureGate('budgetEnforcement'));
      expect(result.current).toBe(false);
    });

    test('should return false for budgetAlerts', () => {
      const { result } = renderHook(() => useFeatureGate('budgetAlerts'));
      expect(result.current).toBe(false);
    });

    test('should return false for ssoIntegration', () => {
      const { result } = renderHook(() => useFeatureGate('ssoIntegration'));
      expect(result.current).toBe(false);
    });

    test('should return false for advancedAnalytics', () => {
      const { result } = renderHook(() => useFeatureGate('advancedAnalytics'));
      expect(result.current).toBe(false);
    });

    test('should return false for auditLogs', () => {
      const { result } = renderHook(() => useFeatureGate('auditLogs'));
      expect(result.current).toBe(false);
    });

    test('should return false for prioritySupport', () => {
      const { result } = renderHook(() => useFeatureGate('prioritySupport'));
      expect(result.current).toBe(false);
    });

    test('should return true for community features', () => {
      const { result: basicAnalytics } = renderHook(() => useFeatureGate('basicAnalytics'));
      const { result: costTracking } = renderHook(() => useFeatureGate('costTracking'));
      const { result: proxyGateway } = renderHook(() => useFeatureGate('proxyGateway'));

      expect(basicAnalytics.current).toBe(true);
      expect(costTracking.current).toBe(true);
      expect(proxyGateway.current).toBe(true);
    });
  });

  describe('Unknown features', () => {
    test('should return false for unknown features in enterprise', () => {
      useEdition.mockReturnValue({ isEnterprise: true });
      const { result } = renderHook(() => useFeatureGate('unknownFeature'));
      expect(result.current).toBe(false);
    });

    test('should return false for unknown features in community', () => {
      useEdition.mockReturnValue({ isEnterprise: false });
      const { result } = renderHook(() => useFeatureGate('unknownFeature'));
      expect(result.current).toBe(false);
    });

    test('should return false for undefined feature name', () => {
      useEdition.mockReturnValue({ isEnterprise: true });
      const { result } = renderHook(() => useFeatureGate(undefined));
      expect(result.current).toBe(false);
    });

    test('should return false for null feature name', () => {
      useEdition.mockReturnValue({ isEnterprise: true });
      const { result } = renderHook(() => useFeatureGate(null));
      expect(result.current).toBe(false);
    });
  });
});
