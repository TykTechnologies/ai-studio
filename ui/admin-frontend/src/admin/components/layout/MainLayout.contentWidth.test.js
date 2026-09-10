import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import MainLayout from './MainLayout';

jest.mock('./AppBar', () => ({
  __esModule: true,
  default: () => <div data-testid="app-bar" />,
}));
jest.mock('./Drawer', () => ({
  __esModule: true,
  default: () => <div data-testid="drawer" />,
}));
jest.mock('../common/SyncStatusBanner', () => ({
  __esModule: true,
  default: () => <div data-testid="sync-banner" />,
}));

const renderLayout = () =>
  render(
    <MemoryRouter initialEntries={['/admin/llms']}>
      <Routes>
        <Route element={<MainLayout hideAppBar />}>
          <Route path="/admin/llms" element={<div data-testid="page">LLM providers</div>} />
        </Route>
      </Routes>
    </MemoryRouter>
  );

describe('Admin MainLayout content width', () => {
  it('wraps routed pages and the sync banner in an xl container without extra gutters', () => {
    renderLayout();
    const page = screen.getByTestId('page');
    const container = page.closest('.MuiContainer-maxWidthXl');
    expect(container).not.toBeNull();
    expect(container).toHaveClass('MuiContainer-disableGutters');
    expect(container).toContainElement(screen.getByTestId('sync-banner'));
    // Centred in the space beside the drawer (MUI Container default)
    expect(container).toHaveStyle({ marginLeft: 'auto', marginRight: 'auto' });
  });
});
