import fs from 'fs';
import path from 'path';
import { P } from './rbac/permissions';
import golden from './nav.golden.json';

// nav.golden.json is the full admin menu from api/nav.go (every feature on,
// Enterprise), written by TestAdminNavGolden. Every page in it must be a
// route in routes.js, and need the permission the menu entry shows.
const routeSource = fs.readFileSync(path.join(__dirname, 'routes.js'), 'utf8');
const routes = [...routeSource.matchAll(/\{\s*path:\s*"([^"]+)"[^}]*?(?:permission:\s*P\.(\w+))?\s*\}/g)].map(
  ([, p, perm]) => ({ path: `/admin/${p}`, permission: perm ? P[perm] : undefined })
);

const pages = (items) =>
  items.flatMap((item) => [...(item.path ? [item] : []), ...pages(item.items || [])]);

const matches = (route, p) => {
  if (route.path.endsWith('/*')) {
    const base = route.path.slice(0, -2);
    return p === base || p.startsWith(`${base}/`);
  }
  const re = new RegExp(`^${route.path.replace(/:[^/]+/g, '[^/]+')}$`);
  return re.test(p);
};

describe('admin navigation manifest', () => {
  it('parses routes.js', () => {
    expect(routes.length).toBeGreaterThan(50);
  });

  it.each(pages(golden).filter((p) => p.path !== '/admin').map((p) => [p.path, p]))(
    '%s is a route with the same permission',
    (p, page) => {
      const route = routes.find((r) => matches(r, p));
      expect(route).toBeDefined();
      if (route.permission) {
        expect(page.permission).toBe(route.permission);
      }
    }
  );
});
