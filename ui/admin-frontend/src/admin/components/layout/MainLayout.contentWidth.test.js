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
  it('renders routed pages full-width, with no max-width container', () => {
    renderLayout();
    const page = screen.getByTestId('page');
    expect(page.closest('.MuiContainer-root')).toBeNull();
    const main = page.closest('main');
    expect(main).not.toBeNull();
    expect(main).toContainElement(screen.getByTestId('sync-banner'));
  });
});
