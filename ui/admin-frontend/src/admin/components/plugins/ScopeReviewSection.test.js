import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import ScopeReviewSection, { SCOPE_DESCRIPTIONS } from './ScopeReviewSection';

describe('ScopeReviewSection group headings', () => {
  it('names the kv, rbac and resource-types groups instead of showing raw slugs', () => {
    render(<ScopeReviewSection scopes={['kv.readwrite', 'rbac.register', 'resource-types.manage']} />);
    expect(screen.getByText('Key-value storage')).toBeInTheDocument();
    expect(screen.getByText('Roles & permissions')).toBeInTheDocument();
    expect(screen.getByText('Resource types')).toBeInTheDocument();
    expect(screen.queryByText('Kv')).not.toBeInTheDocument();
    expect(screen.queryByText('Rbac')).not.toBeInTheDocument();
    expect(screen.queryByText('Resource-types')).not.toBeInTheDocument();
    // Each of the three scopes has its own description, not the generic one.
    expect(screen.getByText(SCOPE_DESCRIPTIONS['kv.readwrite'])).toBeInTheDocument();
    expect(screen.getByText(SCOPE_DESCRIPTIONS['rbac.register'])).toBeInTheDocument();
    expect(screen.getByText(SCOPE_DESCRIPTIONS['resource-types.manage'])).toBeInTheDocument();
    expect(screen.queryByText('Standard access permission')).not.toBeInTheDocument();
  });

  it('names the governance read and resource access groups', () => {
    const scopes = ['resource-access.manage', 'audit.read', 'mcp-servers.read', 'routers.read'];
    render(<ScopeReviewSection scopes={scopes} />);
    expect(screen.getByText('Team access to plugin resources')).toBeInTheDocument();
    expect(screen.getByText('Audit trail')).toBeInTheDocument();
    expect(screen.getByText('MCP servers')).toBeInTheDocument();
    expect(screen.getByText('Routers')).toBeInTheDocument();
    scopes.forEach((scope) => expect(screen.getByText(SCOPE_DESCRIPTIONS[scope])).toBeInTheDocument());
    expect(screen.queryByText('Standard access permission')).not.toBeInTheDocument();
  });

  it('title-cases an unknown category and replaces hyphens with spaces', () => {
    render(<ScopeReviewSection scopes={['foo-bar.x']} />);
    expect(screen.getByText('Foo Bar')).toBeInTheDocument();
    expect(screen.queryByText('Foo-bar')).not.toBeInTheDocument();
  });

  // o4a.19: every scope the plugin SDKs define has its own description, so the
  // install review never falls back to the generic text. The source of truth
  // is checked from Go (pkg/ai_studio_sdk/scope_descriptions_test.go reads
  // AvailableScopes and pkg/gatewayplugin/sdk/service_api.go); this list
  // pins the render.
  it('describes every service scope the SDKs define', () => {
    const sdkScopes = [
      'plugins.read', 'plugins.write', 'plugins.config',
      'llms.read', 'llms.write', 'llms.proxy',
      'tools.read', 'tools.write', 'tools.operations', 'tools.execute',
      'apps.read', 'apps.write', 'apps.lifecycle',
      'kv.read', 'kv.readwrite',
      'analytics.read', 'analytics.write',
      'datasources.read', 'datasources.write', 'datasources.query', 'datasources.embeddings',
      'tags.read', 'tags.write', 'filters.read', 'filters.write',
      'pricing.read', 'pricing.write', 'data-catalogues.read', 'data-catalogues.write',
      'scheduler.manage', 'metadata.read', 'metadata.write', 'notifications.write',
      'resource-types.manage', 'rbac.register', 'resource-access.manage',
      'audit.read', 'mcp-servers.read', 'routers.read',
      'control.send', 'budget.read', 'credentials.validate',
    ];
    sdkScopes.forEach((scope) => expect([scope, SCOPE_DESCRIPTIONS[scope]]).toEqual([scope, expect.any(String)]));

    // The Prompt Compressor's request (o4a.17): no generic text, no raw group slugs.
    render(<ScopeReviewSection scopes={['kv.readwrite', 'control.send', 'pricing.read', 'apps.read', 'llms.read']} />);
    expect(screen.queryByText('Standard access permission')).not.toBeInTheDocument();
    expect(screen.getByText(SCOPE_DESCRIPTIONS['control.send'])).toBeInTheDocument();
    expect(screen.getByText(SCOPE_DESCRIPTIONS['pricing.read'])).toBeInTheDocument();
    expect(screen.getByText('Edge to AI Studio')).toBeInTheDocument();
    expect(screen.getByText('Model prices')).toBeInTheDocument();
  });
});
