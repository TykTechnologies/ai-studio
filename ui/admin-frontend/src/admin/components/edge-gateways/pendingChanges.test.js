import {
  canonicalNamespace,
  changeLink,
  changeTypeLabel,
  formatPushTime,
  groupChanges,
  relativeTime,
  sameNamespace,
  summarizePending,
} from './pendingChanges';

describe('pendingChanges helpers', () => {
  const now = new Date('2026-09-14T13:20:00Z');

  it('names change types with the product words', () => {
    expect(changeTypeLabel('llm')).toBe('LLM providers');
    expect(changeTypeLabel('datasource')).toBe('Data sources');
    expect(changeTypeLabel('model_price')).toBe('Model prices');
    expect(changeTypeLabel('semantic_router')).toBe('Semantic routers');
    expect(changeTypeLabel('access_token')).toBe('Access tokens');
    expect(changeTypeLabel('something_new')).toBe('something_new');
  });

  it('links to the admin page when there is one', () => {
    expect(changeLink({ type: 'llm', id: 3, change: 'updated' })).toBe('/admin/llms/3');
    expect(changeLink({ type: 'model_router', id: 'r1', change: 'created' })).toBe('/admin/model-routers/r1');
    expect(changeLink({ type: 'semantic_router', id: 5, change: 'updated' })).toBe('/admin/semantic-routers/5');
    expect(changeLink({ type: 'access_token', id: 9, change: 'created' })).toBeNull();
    expect(changeLink({ type: 'app', id: 4, change: 'deleted' })).toBeNull();
  });

  it('groups by type in display order', () => {
    const groups = groupChanges([
      { type: 'tool', id: 1, name: 'T' },
      { type: 'llm', id: 2, name: 'L' },
      { type: 'mystery', id: 3, name: 'M' },
      { type: 'llm', id: 4, name: 'L2' },
    ]);
    expect(groups.map((g) => g.label)).toEqual(['LLM providers', 'Tools', 'mystery']);
    expect(groups[0].changes.map((c) => c.id)).toEqual([2, 4]);
  });

  it('formats push times as HH:MM today and with the day otherwise', () => {
    const today = formatPushTime('2026-09-14T13:12:00Z', now);
    expect(today).toMatch(/^\d{2}:\d{2}$/);
    const yesterday = formatPushTime('2026-09-13T13:12:00Z', now);
    expect(yesterday).toMatch(/Sep/);
    expect(formatPushTime(null, now)).toBeNull();
  });

  it('describes when a change happened relative to now', () => {
    expect(relativeTime('2026-09-14T13:19:50Z', now)).toBe('just now');
    expect(relativeTime('2026-09-14T13:15:00Z', now)).toBe('5 minutes ago');
    expect(relativeTime('2026-09-14T10:20:00Z', now)).toBe('3 hours ago');
    expect(relativeTime('2026-09-13T13:20:00Z', now)).toBe('yesterday');
    expect(relativeTime('2026-09-10T13:20:00Z', now)).toBe('4 days ago');
  });

  it('summarises the count against the last push', () => {
    const at = '2026-09-14T13:12:00Z';
    const time = formatPushTime(at, now);
    expect(summarizePending({ total: 12, lastPushAt: at }, now)).toBe(`12 changes since the last push at ${time}`);
    expect(summarizePending({ total: 1, lastPushAt: at }, now)).toBe(`1 change since the last push at ${time}`);
    expect(summarizePending({ total: 0, lastPushAt: at }, now)).toBe(`Nothing has changed since the last push (${time})`);
    expect(summarizePending({ total: 3, lastPushAt: null }, now)).toBe('3 changes (never pushed)');
    expect(summarizePending({ total: 0, lastPushAt: null }, now)).toBe('Nothing has changed (no push recorded yet)');
    expect(summarizePending({ total: 3, lastPushAt: null, baseline: 'none' }, now)).toBe('3 changes (never pushed)');
  });

  it('measures from the edge ack when no push was recorded but an edge is in sync', () => {
    const at = '2026-09-14T13:12:00Z';
    const time = formatPushTime(at, now);
    const acked = { total: 3, lastPushAt: null, since: at, baseline: 'edge_ack' };
    expect(summarizePending(acked, now)).toBe(`3 changes since the last sync at ${time}`);
    expect(summarizePending({ ...acked, total: 1 }, now)).toBe(`1 change since the last sync at ${time}`);
    expect(summarizePending({ ...acked, total: 0 }, now)).toBe(`Nothing has changed since the last sync (${time})`);
    // A recorded push still reads as a push.
    expect(summarizePending({ total: 2, lastPushAt: at, since: at, baseline: 'push' }, now)).toBe(
      `2 changes since the last push at ${time}`
    );
  });

  it('treats "", "global" and "default" as the same namespace', () => {
    expect(canonicalNamespace('')).toBe('default');
    expect(canonicalNamespace(undefined)).toBe('default');
    expect(canonicalNamespace(null)).toBe('default');
    expect(canonicalNamespace('global')).toBe('default');
    expect(canonicalNamespace('default')).toBe('default');
    expect(canonicalNamespace(' eu-west ')).toBe('eu-west');
    expect(sameNamespace('', 'default')).toBe(true);
    expect(sameNamespace('global', 'default')).toBe(true);
    expect(sameNamespace(undefined, 'global')).toBe(true);
    expect(sameNamespace('eu-west', 'eu-west')).toBe(true);
    expect(sameNamespace('eu-west', 'default')).toBe(false);
    expect(sameNamespace('eu-west', '')).toBe(false);
  });
});
