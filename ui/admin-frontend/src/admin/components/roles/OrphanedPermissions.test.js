import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react';
import '@testing-library/jest-dom';
import OrphanedPermissions from './OrphanedPermissions';
import usePermissionCatalogue from '../../hooks/usePermissionCatalogue';

jest.mock('../../hooks/usePermissionCatalogue');

const catalogue = {
  loading: false,
  byKey: new Map([
    ['llms', { key: 'llms' }],
    ['plugin:com.example.installed', { key: 'plugin:com.example.installed' }],
  ]),
};

describe('OrphanedPermissions', () => {
  beforeEach(() => {
    usePermissionCatalogue.mockReturnValue(catalogue);
  });

  it('renders nothing when every held permission is in the catalogue', () => {
    render(<OrphanedPermissions permissions={new Set(['llms:read', 'plugin:com.example.installed:write'])} />);
    expect(screen.queryByTestId('orphaned-permissions')).toBeNull();
  });

  it('lists grants whose plugin is gone and lets the editor remove them', () => {
    const onRemove = jest.fn();
    render(
      <OrphanedPermissions
        permissions={new Set(['llms:read', 'plugin:com.example.gone:write', 'plugin:com.example.gone:read'])}
        onRemove={onRemove}
      />
    );
    expect(screen.getByText('Not installed')).toBeInTheDocument();
    expect(screen.getByText('plugin:com.example.gone:read')).toBeInTheDocument();
    expect(screen.getByText('plugin:com.example.gone:write')).toBeInTheDocument();
    expect(screen.queryByText('llms:read')).toBeNull();
    // Chips render sorted, so the first delete icon belongs to ":read".
    fireEvent.click(screen.getAllByTestId('CancelIcon')[0]);
    expect(onRemove).toHaveBeenCalledWith('plugin:com.example.gone:read');
  });

  it('is read-only without onRemove and ignores the wildcard', () => {
    render(<OrphanedPermissions permissions={['*', 'plugin:com.example.gone:read']} />);
    expect(screen.getByText('plugin:com.example.gone:read')).toBeInTheDocument();
    expect(screen.queryByTestId('CancelIcon')).toBeNull();
  });
});
