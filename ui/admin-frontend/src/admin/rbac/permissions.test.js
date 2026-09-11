import { P, hasPermission, hasPluginGrant, isPluginPermission, ACTIONS } from './permissions';

describe('permissions vocabulary', () => {
  it('knows the five actions and the publish constants', () => {
    expect(ACTIONS).toEqual(['read', 'write', 'delete', 'execute', 'publish']);
    expect(P.LLMS_PUBLISH).toBe('llms:publish');
    expect(P.PLUGINS_PUBLISH).toBe('plugins:publish');
  });

  it('implies read from any action, but never write from publish', () => {
    const held = new Set([P.LLMS_PUBLISH]);
    expect(hasPermission(held, P.LLMS_READ)).toBe(true);
    expect(hasPermission(held, P.LLMS_PUBLISH)).toBe(true);
    expect(hasPermission(held, P.LLMS_WRITE)).toBe(false);
  });

  it('applies the umbrella rule: plugins:execute grants every plugin permission', () => {
    const key = 'plugin:com.example.assets';
    expect(isPluginPermission(`${key}:read`)).toBe(true);
    expect(isPluginPermission(P.PLUGINS_READ)).toBe(false);

    const execute = new Set([P.PLUGINS_EXECUTE]);
    expect(hasPermission(execute, `${key}:write`)).toBe(true);
    expect(hasPermission(execute, `${key}:assets:publish`)).toBe(true);
    expect(hasPermission(execute, P.PLUGINS_WRITE)).toBe(false);

    const read = new Set([P.PLUGINS_READ]);
    expect(hasPermission(read, `${key}:read`)).toBe(false);

    const perPlugin = new Set([`${key}:write`]);
    expect(hasPermission(perPlugin, `${key}:read`)).toBe(true);
    expect(hasPermission(perPlugin, `${key}:assets:read`)).toBe(false);
    expect(hasPermission(perPlugin, P.PLUGINS_READ)).toBe(false);
  });

  it('detects any plugin grant for the plugin pages route', () => {
    expect(hasPluginGrant(new Set())).toBe(false);
    expect(hasPluginGrant(new Set([P.PLUGINS_READ]))).toBe(false);
    expect(hasPluginGrant(new Set([P.PLUGINS_EXECUTE]))).toBe(true);
    expect(hasPluginGrant(new Set(['plugin:com.example.assets:read']))).toBe(true);
    expect(hasPluginGrant(new Set(['*']))).toBe(true);
  });
});
