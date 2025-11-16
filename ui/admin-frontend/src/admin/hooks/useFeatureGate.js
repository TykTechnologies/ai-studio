import { useEdition } from "../context/EditionContext";

/**
 * Hook for checking if a feature is available in the current edition
 *
 * @param {string} featureName - The name of the feature to check
 * @returns {boolean} - Whether the feature is available
 *
 * @example
 * const isBudgetAvailable = useFeatureGate('budgetEnforcement');
 *
 * Available features:
 * - budgetEnforcement: Budget enforcement and alerts (Enterprise only)
 * - ssoIntegration: SSO integration (Enterprise only)
 * - advancedAnalytics: Advanced analytics dashboard (Enterprise only)
 * - auditLogs: Detailed audit logging (Enterprise only)
 */
const useFeatureGate = (featureName) => {
  const { isEnterprise } = useEdition();

  // Define feature availability mapping
  const features = {
    // Budget features
    budgetEnforcement: isEnterprise,
    budgetAlerts: isEnterprise,

    // Future enterprise features can be added here
    ssoIntegration: isEnterprise,
    advancedAnalytics: isEnterprise,
    auditLogs: isEnterprise,
    prioritySupport: isEnterprise,

    // Community features (always available)
    basicAnalytics: true,
    costTracking: true,
    proxyGateway: true,
  };

  // Return feature availability, default to false if feature not defined
  return features[featureName] !== undefined ? features[featureName] : false;
};

export default useFeatureGate;
