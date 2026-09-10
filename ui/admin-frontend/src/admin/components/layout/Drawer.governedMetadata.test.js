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

const expandGovernance = async () => {
  fireEvent.click(await screen.findByText('Governance'));
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 350));
  });
};

describe('Drawer governed metadata navigation', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    pluginLoaderService.getSidebarMenuItems.mockResolvedValue([]);
    Object.defineProperty(window, 'localStorage', {
      value: { getItem: jest.fn(() => null), setItem: jest.fn(), removeItem: jest.fn(), clear: jest.fn() },
      configurable: true,
    });
  });

  it('shows the three metadata entries next to Compliance in Enterprise', async () => {
    useAdminData.mockReturnValue(adminData(true));
    renderDrawer();
    await expandGovernance();
    for (const [text, path] of [
      ['Compliance', '/admin/compliance'],
      ['Metadata schemas', '/admin/metadata/schemas'],
      ['Vocabularies', '/admin/metadata/vocabularies'],
      ['Metadata compliance', '/admin/metadata/compliance'],
    ]) {
      const item = screen.getByText(text);
      expect(item).toBeInTheDocument();
      const link = item.closest('a');
      if (link) {
        expect(link).toHaveAttribute('href', path);
      }
    }
  });

  it('hides them in Community Edition', async () => {
    useAdminData.mockReturnValue(adminData(false));
    renderDrawer();
    await expandGovernance();
    expect(screen.getByText('Secrets')).toBeInTheDocument();
    expect(screen.queryByText('Metadata schemas')).not.toBeInTheDocument();
    expect(screen.queryByText('Vocabularies')).not.toBeInTheDocument();
    expect(screen.queryByText('Metadata compliance')).not.toBeInTheDocument();
  });
});
