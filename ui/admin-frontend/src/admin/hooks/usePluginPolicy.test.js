import { renderHook, waitFor } from '@testing-library/react';
import apiClient from '../utils/apiClient';
import usePluginPolicy, { resetPluginPolicyCache } from './usePluginPolicy';

jest.mock('../utils/apiClient');

describe('usePluginPolicy', () => {
  beforeEach(() => {
    resetPluginPolicyCache();
    apiClient.get.mockReset();
  });

  test('reads the policy from the authenticated plugin-policy endpoint', async () => {
    apiClient.get.mockResolvedValue({ data: { data: { plugin_allowed_sources: ['oci'], oci_require_signature: true } } });

    const { result } = renderHook(() => usePluginPolicy());
    await waitFor(() => expect(result.current.policy).not.toBeNull());

    expect(apiClient.get).toHaveBeenCalledWith('/plugin-policy');
    expect(result.current.policy).toEqual({ plugin_allowed_sources: ['oci'], oci_require_signature: true });
  });

  test('stays null when the policy cannot be read', async () => {
    apiClient.get.mockRejectedValue(new Error('403'));

    const { result } = renderHook(() => usePluginPolicy());
    await waitFor(() => expect(apiClient.get).toHaveBeenCalled());
    expect(result.current.policy).toBeNull();
  });
});
