import React from 'react';
import LockedFeaturePanel, { LockedFeatureAction } from './LockedFeaturePanel';

const EnterpriseFeatureBadge = ({
  feature = "Enterprise Feature",
  description = "This feature is only available in the Enterprise Edition.",
  showUpgradeButton = true
}) => (
  <LockedFeaturePanel
    icon="lock"
    title={feature}
    description={description}
    action={
      showUpgradeButton ? (
        <LockedFeatureAction href="https://tyk.io/enterprise">
          Learn More About Enterprise
        </LockedFeatureAction>
      ) : null
    }
  />
);

export default EnterpriseFeatureBadge;
