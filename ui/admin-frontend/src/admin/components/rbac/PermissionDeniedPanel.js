import React from 'react';
import { useNavigate } from 'react-router-dom';
import LockedFeaturePanel, { LockedFeatureAction } from '../common/LockedFeaturePanel';
import { toArray } from '../../rbac/permissions';
import usePermissionCatalogue from '../../hooks/usePermissionCatalogue';

/**
 * Shown in place of a page the user's role does not allow.
 */
const PermissionDeniedPanel = ({ required, title, description }) => {
  const navigate = useNavigate();
  // Catalogue labels ("LLM providers: write") when loaded, static fallback otherwise.
  const { label } = usePermissionCatalogue();
  const perms = toArray(required);
  const labels = perms.map(label).filter(Boolean);
  const text =
    description ||
    (labels.length > 0
      ? `This page requires the ${labels.join(' or ')} permission. Ask an administrator to assign you a role that includes it.`
      : 'Ask an administrator to assign you a role that includes access to this page.');

  return (
    <LockedFeaturePanel
      icon="lock"
      testId="permission-denied-panel"
      title={title || "You don't have permission to view this page"}
      description={text}
      action={
        <LockedFeatureAction onClick={() => navigate('/admin')}>
          Back to overview
        </LockedFeatureAction>
      }
    />
  );
};

export default PermissionDeniedPanel;
