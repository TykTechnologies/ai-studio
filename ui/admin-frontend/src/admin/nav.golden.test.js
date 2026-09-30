import fs from 'fs';
import path from 'path';
import { P } from './rbac/permissions';
import golden from './nav.golden.json';

// nav.golden.json holds the menus from api/nav.go with every feature on,
// written by TestNavGolden. Every page in them must be a console route; an
// admin page must also need the permission its menu entry shows.
const read = (file) => fs.readFileSync(path.join(__dirname, file), 'utf8');

const adminRoutes = [...read('routes.js').matchAll(/\{\s*path:\s*"([^"]+)"[^}]*?(?:permission:\s*P\.(\w+))?\s*\}/g)].map(
  ([, p, perm]) => ({ path: `/admin/${p}`, permission: perm ? P[perm] : undefined })
);
const jsxRoutes = (file, prefix) =>
  [...read(file).matchAll(/<Route\s+path="([^"]+)"/g)].map(([, p]) => ({ path: `${prefix}${p === '/' ? '' : p}` }));
const portalRoutes = jsxRoutes('../routes/PortalRoutes.js', '/portal');
const chatRoutes = jsxRoutes('../routes/ChatRoutes.js', '/chat');

const pages = (items) =>
  items.flatMap((item) => [...(item.path ? [item] : []), ...pages(item.items || [])]);

const matches = (route, p) => {
  const pathOnly = p.split('?')[0];
  if (route.path.endsWith('/*')) {
    const base = route.path.slice(0, -2);
    return pathOnly === base || pathOnly.startsWith(`${base}/`);
  }
  return new RegExp(`^${route.path.replace(/:[^/]+/g, '[^/]+')}$`).test(pathOnly);
};

describe('navigation manifest', () => {
  it('parses the route files', () => {
    expect(adminRoutes.length).toBeGreaterThan(50);
    expect(portalRoutes.length).toBeGreaterThan(10);
    expect(chatRoutes.length).toBeGreaterThan(3);
  });

  it.each(pages(golden.admin).filter((p) => p.path !== '/admin').map((p) => [p.path, p]))(
    'admin %s is a route with the same permission',
    (p, page) => {
      const route = adminRoutes.find((r) => matches(r, p));
      expect(route).toBeDefined();
      if (route.permission) {
        expect(page.permission).toBe(route.permission);
      }
    }
  );

  it.each(pages(golden.portal).map((p) => [p.path]))('portal %s is a route', (p) => {
    expect(portalRoutes.find((r) => matches(r, p))).toBeDefined();
  });

  it.each(pages(golden.chat).map((p) => [p.path]))('chat %s is a route', (p) => {
    expect(chatRoutes.find((r) => matches(r, p))).toBeDefined();
  });
});
