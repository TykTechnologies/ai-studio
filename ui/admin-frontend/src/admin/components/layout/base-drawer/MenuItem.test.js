import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import { ThemeProvider } from '@mui/material/styles';
import BaseDrawer from './BaseDrawer';
import adminTheme from '../../../theme';

// The highlight is derived from the URL alone: the longest matching item
// path wins, and only that item (plus the groups above it) lights up.
const menuItems = [
  { id: 'overview', text: 'Overview', path: '/admin', exact: true },
  {
    id: 'access',
    text: 'Access',
    subItems: [{ id: 'users', text: 'Users', path: '/admin/users' }],
  },
  {
    id: 'catalogs',
    text: 'Catalogs',
    subItems: [
      { id: 'catalog-llms', text: 'LLM providers', path: '/admin/catalogs/llms' },
    ],
  },
  {
    id: 'llm-management',
    text: 'LLM management',
    subItems: [{ id: 'llms', text: 'LLM providers', path: '/admin/llms' }],
  },
  {
    id: 'ai-portal',
    text: 'AI Portal',
    subItems: [
      { id: 'portal-apps', text: 'Apps', path: '/admin/apps' },
      { id: 'edge-gateways', text: 'Edge Gateways', path: '/admin/edge-gateways' },
    ],
  },
];

const renderAt = (path) =>
  render(
    <MemoryRouter initialEntries={[path]}>
      <ThemeProvider theme={adminTheme}>
        <BaseDrawer id="t" menuItems={menuItems} />
      </ThemeProvider>
    </MemoryRouter>
  );

const selectedIds = () =>
  Array.from(document.querySelectorAll('[data-nav-id]'))
    .filter((el) => el.classList.contains('Mui-selected') || el.getAttribute('aria-current') === 'page')
    .map((el) => el.getAttribute('data-nav-id'));

beforeEach(() => {
  Object.defineProperty(window, 'localStorage', {
    value: { getItem: jest.fn(() => null), setItem: jest.fn(), removeItem: jest.fn(), clear: jest.fn() },
    configurable: true,
  });
});

describe('drawer highlighting follows the URL', () => {
  it('highlights only Overview on /admin', () => {
    renderAt('/admin');
    expect(selectedIds()).toEqual(['overview']);
    expect(screen.getByRole('link', { name: 'Overview' })).toHaveAttribute('aria-current', 'page');
  });

  it('highlights Apps (and its group), not Overview, on a detail page', () => {
    renderAt('/admin/apps/1');
    expect(selectedIds()).toEqual(['ai-portal', 'portal-apps']);
    expect(screen.getByRole('link', { name: 'Overview' })).not.toHaveAttribute('aria-current');
    expect(screen.getByRole('link', { name: 'Apps' })).toHaveAttribute('aria-current', 'page');
    expect(screen.getByRole('link', { name: 'Edge Gateways' })).not.toHaveAttribute('aria-current');
  });

  it('prefers the catalog entry on /admin/catalogs/llms/2 over LLM management', () => {
    renderAt('/admin/catalogs/llms/2');
    expect(selectedIds()).toEqual(['catalogs', 'catalog-llms']);
    // The group with the match is opened, the other one is left closed.
    expect(screen.getByRole('link', { name: 'LLM providers' })).toHaveAttribute('href', '/admin/catalogs/llms');
  });

  it('highlights the LLM management entry on /admin/llms/2', () => {
    renderAt('/admin/llms/2');
    expect(selectedIds()).toEqual(['llm-management', 'llms']);
    expect(screen.getByRole('link', { name: 'LLM providers' })).toHaveAttribute('href', '/admin/llms');
  });

  it('highlights nothing on a page without a menu entry', () => {
    renderAt('/admin/somewhere-else');
    expect(selectedIds()).toEqual([]);
  });
});
