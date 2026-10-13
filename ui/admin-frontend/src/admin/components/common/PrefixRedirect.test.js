import React from 'react';
import { render, screen } from '@testing-library/react';
import '@testing-library/jest-dom';
import fs from 'fs';
import path from 'path';
import { MemoryRouter, Routes, Route, useLocation } from 'react-router-dom';
import PrefixRedirect from './PrefixRedirect';

// Data catalogs moved from /admin/catalogs/data to /admin/catalogs/datasources
// (matching catalogs/llms and catalogs/tools); old links must still land.

const Where = () => {
  const { pathname, search, hash } = useLocation();
  return <div data-testid="where">{`${pathname}${search}${hash}`}</div>;
};

const at = (entry) =>
  render(
    <MemoryRouter initialEntries={[entry]}>
      <Routes>
        <Route path="/admin/catalogs/data/*" element={<PrefixRedirect to="catalogs/datasources" />} />
        <Route path="/admin/catalogs/datasources/*" element={<Where />} />
      </Routes>
    </MemoryRouter>,
  );

describe('catalogs/data redirect', () => {
  test.each([
    ['/admin/catalogs/data', '/admin/catalogs/datasources'],
    ['/admin/catalogs/data/', '/admin/catalogs/datasources'],
    ['/admin/catalogs/data/4', '/admin/catalogs/datasources/4'],
    ['/admin/catalogs/data/edit/4', '/admin/catalogs/datasources/edit/4'],
    ['/admin/catalogs/data/new?from=x#top', '/admin/catalogs/datasources/new?from=x#top'],
  ])('%s -> %s', async (from, to) => {
    at(from);
    expect(await screen.findByTestId('where')).toHaveTextContent(to);
  });

  test('routes.js serves data catalogs under catalogs/datasources and keeps catalogs/data as a redirect', () => {
    const src = fs.readFileSync(path.join(__dirname, '../../routes.js'), 'utf8');
    const paths = [...src.matchAll(/path:\s*"(catalogs\/data[^"]*)"/g)].map(([, p]) => p);
    expect(paths.sort()).toEqual([
      'catalogs/data/*',
      'catalogs/datasources',
      'catalogs/datasources/:id',
      'catalogs/datasources/edit/:id',
      'catalogs/datasources/new',
    ]);
    expect(src).toMatch(/path: "catalogs\/data\/\*", element: <PrefixRedirect to="catalogs\/datasources" \/>/);
  });
});
