import React from 'react';
import { render as rtlRender, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { ThemeProvider } from '@mui/material/styles';
import adminTheme from '../../theme';
import EffectivePermissionsList, { describeSources } from './EffectivePermissionsList';
import usePermissionCatalogue from '../../hooks/usePermissionCatalogue';

jest.mock('../../hooks/usePermissionCatalogue');

const catalogue = {
  loading: false,
  actions: ['read', 'write', 'delete', 'execute'],
  byKey: new Map([['llms', { key: 'llms', label: 'LLM providers' }]]),
  grouped: [
    {
      group: 'LLM management',
      resources: [
        { key: 'llms', label: 'LLM providers', group: 'LLM management', actions: ['read', 'write', 'delete'] },
      ],
    },
  ],
};

const sources = {
  'llms:read': [
    { role_id: 3, role_name: 'Editor', via: 'group', group_id: 9, group_name: 'Platform' },
    { role_id: 5, role_name: 'Viewer', via: 'direct' },
  ],
  'llms:write': [{ role_id: 3, role_name: 'Editor', via: 'group', group_id: 9, group_name: 'Platform' }],
};

const render = (ui) => rtlRender(<ThemeProvider theme={adminTheme}>{ui}</ThemeProvider>);

describe('EffectivePermissionsList', () => {
  beforeEach(() => {
    usePermissionCatalogue.mockReturnValue(catalogue);
  });

  it('groups permissions by resource and renders an action chip each', () => {
    render(<EffectivePermissionsList permissions={['llms:read', 'llms:write']} />);
    expect(screen.getByText('LLM management')).toBeInTheDocument();
    expect(screen.getByText('LLM providers')).toBeInTheDocument();
    expect(screen.getByText('read')).toBeInTheDocument();
    expect(screen.getByText('write')).toBeInTheDocument();
  });

  it('names the role and team each permission came from', () => {
    render(<EffectivePermissionsList permissions={['llms:read', 'llms:write']} sources={sources} />);
    // MUI Tooltip exposes a string title as the chip's accessible label.
    expect(screen.getByLabelText('from Editor via team Platform')).toHaveTextContent('write');
    expect(screen.getByLabelText('from Editor via team Platform; from Viewer (direct)')).toHaveTextContent('read');
  });

  it('renders plain chips when a permission has no source', () => {
    render(<EffectivePermissionsList permissions={['llms:read']} sources={{}} />);
    expect(screen.getByText('read')).toBeInTheDocument();
    expect(screen.queryByLabelText(/from /)).toBeNull();
  });

  it('shows the wildcard line without any provenance', () => {
    render(<EffectivePermissionsList permissions={['*']} sources={{ '*': [{ role_name: 'Owner', via: 'direct' }] }} />);
    expect(screen.getByText('Full administrator access (all permissions).')).toBeInTheDocument();
  });
});

describe('describeSources', () => {
  it('formats direct and team-granted sources', () => {
    expect(describeSources([{ role_name: 'Viewer', via: 'direct' }])).toBe('from Viewer (direct)');
    expect(describeSources([{ role_name: 'Editor', via: 'group', group_id: 9, group_name: 'Platform' }])).toBe(
      'from Editor via team Platform'
    );
    expect(describeSources([{ role_id: 4, via: 'group', group_id: 9 }])).toBe('from role #4 via team #9');
    expect(describeSources([])).toBe('');
    expect(describeSources(undefined)).toBe('');
  });
});
