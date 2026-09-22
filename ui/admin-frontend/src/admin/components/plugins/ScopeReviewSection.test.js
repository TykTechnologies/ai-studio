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

  it('title-cases an unknown category and replaces hyphens with spaces', () => {
    render(<ScopeReviewSection scopes={['foo-bar.x']} />);
    expect(screen.getByText('Foo Bar')).toBeInTheDocument();
    expect(screen.queryByText('Foo-bar')).not.toBeInTheDocument();
  });
});
