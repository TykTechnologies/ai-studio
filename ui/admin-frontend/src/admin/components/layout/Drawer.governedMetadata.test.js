import React from 'react';
import { render, screen, fireEvent, act } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import Drawer from './Drawer';
import useAdminData from '../../hooks/useAdminData';
import adminTheme from '../../theme';

jest.mock('../../hooks/useAdminData');
jest.mock('../../services/pluginLoaderService', () => ({
  __esModule: true,
  default: { getSidebarMenuItems: jest.fn(), initialize: jest.fn() },
}));
jest.mock('../../../components/common/Icon', () => ({
  __esModule: true,
  default: ({ name }) => <span data-testid={`icon-${name}`} />,
}));

const pluginLoaderService = require('../../services/pluginLoaderService').default;

const adminData = (isEnterprise) => ({
  features: { feature_portal: true, feature_chat: true, feature_gateway: true, feature_groups: false },
  uiOptions: {},
  config: { is_enterprise: isEnterprise },
  loading: false,
  error: null,
});

const renderDrawer = () =>
  render(
    <ThemeProvider theme={adminTheme}>
      <MemoryRouter>
        <Drawer />
      </MemoryRouter>
    </ThemeProvider>
  );

const expandGroup = async (label) => {
  fireEvent.click(await screen.findByText(label));
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 350));
  });
};

describe('Drawer admin navigation groups', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pluginLoaderService.getSidebarMenuItems.mockResolvedValue([]);
    Object.defineProperty(window, 'localStorage', {
      value: { getItem: jest.fn(() => null), setItem: jest.fn(), removeItem: jest.fn(), clear: jest.fn() },
      configurable: true,
    });
  });

  const expectLinks = (pairs) => {
    for (const [text, path] of pairs) {
      const item = screen.getByText(text);
      expect(item).toBeInTheDocument();
      const link = item.closest('a');
      if (link) {
        expect(link).toHaveAttribute('href', path);
      }
    }
  };

  it('groups the Enterprise governance pages under Governance', async () => {
    useAdminData.mockReturnValue(adminData(true));
    renderDrawer();
    await expandGroup('Governance');
    expectLinks([
      ['Compliance overview', '/admin/compliance'],
      ['Audit trail', '/admin/audit'],
      ['Metadata schemas', '/admin/metadata/schemas'],
      ['Metadata vocabularies', '/admin/metadata/vocabularies'],
      ['Metadata coverage', '/admin/metadata/compliance'],
    ]);
    // Access and Settings pages no longer live under Governance.
    expect(screen.queryByText('Users')).not.toBeInTheDocument();
    expect(screen.queryByText('Secrets')).not.toBeInTheDocument();
  });

  it('puts identity pages under Access and system pages under Settings', async () => {
    useAdminData.mockReturnValue(adminData(true));
    renderDrawer();
    await expandGroup('Access');
    expectLinks([['Users', '/admin/users']]);
    await expandGroup('Settings');
    expectLinks([
      ['Secrets', '/admin/secrets'],
      ['Branding', '/admin/branding'],
    ]);
  });

  it('hides the Governance group entirely in Community Edition', async () => {
    useAdminData.mockReturnValue(adminData(false));
    renderDrawer();
    expect(await screen.findByText('Access')).toBeInTheDocument();
    expect(screen.getByText('Settings')).toBeInTheDocument();
    expect(screen.queryByText('Governance')).not.toBeInTheDocument();
    await expandGroup('Settings');
    expect(screen.getByText('Secrets')).toBeInTheDocument();
    expect(screen.queryByText('Metadata schemas')).not.toBeInTheDocument();
    expect(screen.queryByText('Metadata vocabularies')).not.toBeInTheDocument();
    expect(screen.queryByText('Metadata coverage')).not.toBeInTheDocument();
  });
});
