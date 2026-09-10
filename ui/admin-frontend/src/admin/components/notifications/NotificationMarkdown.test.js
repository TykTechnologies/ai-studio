import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import { MemoryRouter } from 'react-router-dom';
import NotificationMarkdown from './NotificationMarkdown';

const renderMd = (md) =>
  render(
    <MemoryRouter>
      <NotificationMarkdown>{md}</NotificationMarkdown>
    </MemoryRouter>
  );

describe('NotificationMarkdown', () => {
  it('renders markdown and routes internal links through the router', () => {
    renderMd('**alice** asked for [Triage Agent](/portal/assets/ast_1) and [docs](https://example.com)');
    expect(screen.getByText('alice').tagName).toBe('STRONG');
    expect(screen.getByRole('link', { name: 'Triage Agent' })).toHaveAttribute('href', '/portal/assets/ast_1');
    const external = screen.getByRole('link', { name: 'docs' });
    expect(external).toHaveAttribute('target', '_blank');
    expect(external).toHaveAttribute('rel', 'noopener noreferrer');
  });

  it('never renders raw HTML from plugin-authored content', () => {
    const { container } = renderMd('hello <img src=x onerror="alert(1)"> <script>alert(2)</script>');
    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector('script')).toBeNull();
    expect(container.textContent).toContain('alert(2)');
  });

  it('drops script-capable link destinations', () => {
    renderMd('[go](javascript:alert(1))');
    const link = screen.getByText('go');
    expect(link.getAttribute('href') || '').not.toMatch(/javascript:/i);
  });
});
