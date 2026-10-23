import { useEffect, useState } from 'react';
import apiClient from '../utils/apiClient';

// The installation's plugin policy changes only with a restart, so it is
// fetched once per page load.
let policyRequest = null;

const fetchPluginPolicy = () => {
  if (!policyRequest) {
    policyRequest = apiClient
      .get('/plugin-policy')
      .then((response) => response.data?.data || null)
      .catch((error) => {
        policyRequest = null; // try again next time
        throw error;
      });
  }
  return policyRequest;
};

// usePluginPolicy returns the plugin policy from GET /plugin-policy: the
// plugin deployment types this installation accepts
// (plugin_allowed_sources) and whether OCI plugins must be signed
// (oci_require_signature). null until loaded, or if it cannot be read, in
// which case the console offers every source and the backend still refuses
// what the policy does not allow.
const usePluginPolicy = () => {
  const [policy, setPolicy] = useState(null);

  useEffect(() => {
    let active = true;
    fetchPluginPolicy()
      .then((data) => {
        if (active) setPolicy(data);
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, []);

  return { policy };
};

// resetPluginPolicyCache forgets the fetched policy (tests).
export const resetPluginPolicyCache = () => {
  policyRequest = null;
};

export default usePluginPolicy;
