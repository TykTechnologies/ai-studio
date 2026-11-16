import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import adminTheme from '../../theme';
import Workspaces from './Workspaces';
import { listWorkspaces } from '../../services/workspacesService';
import { EnterpriseFeatureError } from '../../utils/apiErrors';

jest.mock('../../services/workspacesService', () => ({ listWorkspaces: jest.fn() }));
jest.mock('../../../components/common/Icon', () => ({
  __esModule: true,
  default: ({ name }) => <span data-testid={`icon-${name}`} />,
}));

const renderPage = () =>
  render(
    <ThemeProvider theme={adminTheme}>
      <MemoryRouter>
        <Workspaces />
      </MemoryRouter>
    </ThemeProvider>
  );

describe('Workspaces page', () => {
  it('shows the upgrade prompt in Community Edition (402)', async () => {
    listWorkspaces.mockRejectedValue(new EnterpriseFeatureError('Enterprise feature'));
    renderPage();
    expect(await screen.findByText(/Group LLMs, Apps, tools/)).toBeInTheDocument();
  });

  it('lists the Workspaces, the Default marked', async () => {
    listWorkspaces.mockResolvedValue([
      { id: '1', attributes: { name: 'Default', is_default: true } },
      { id: '2', attributes: { name: 'Ops', description: 'Operations' } },
    ]);
    renderPage();
    expect(await screen.findByText('Ops')).toBeInTheDocument();
    expect(screen.getByText('Operations')).toBeInTheDocument();
    expect(screen.getAllByText('Default')).toHaveLength(2);
  });
});
