import { listAll, subscribeListTruncated, LIST_ALL_PAGE_SIZE, LIST_ALL_MAX_ROWS } from './listAll';

const rows = (n, offset = 0) => Array.from({ length: n }, (_, i) => ({ id: String(offset + i + 1) }));

// A client serving `total` rows the way the list endpoints do.
const pagedClient = (total, { bare = false, headers = true } = {}) => ({
  get: jest.fn(async (path, { params }) => {
    const start = (params.page - 1) * params.page_size;
    const pageRows = rows(Math.max(0, Math.min(params.page_size, total - start)), start);
    return {
      data: bare ? pageRows : { data: pageRows },
      headers: headers ? { 'x-total-pages': String(Math.ceil(total / params.page_size)) } : {},
    };
  }),
});

describe('listAll', () => {
  it('returns a short list in one bounded request', async () => {
    const client = pagedClient(12);
    const res = await listAll(client, '/tools');
    expect(res.data.data).toHaveLength(12);
    expect(res.truncated).toBe(false);
    expect(client.get).toHaveBeenCalledTimes(1);
    expect(client.get).toHaveBeenCalledWith('/tools', { params: { page: 1, page_size: LIST_ALL_PAGE_SIZE } });
  });

  it('walks every page, past the default page size of 10', async () => {
    const total = LIST_ALL_PAGE_SIZE * 2 + 5;
    const client = pagedClient(total);
    const res = await listAll(client, '/tools', { sort_by: 'name' });
    expect(res.data.data).toHaveLength(total);
    expect(res.data.data[total - 1].id).toBe(String(total));
    expect(client.get).toHaveBeenCalledTimes(3);
    expect(client.get).toHaveBeenLastCalledWith('/tools', { params: { sort_by: 'name', page: 3, page_size: LIST_ALL_PAGE_SIZE } });
  });

  it('stops on the last full page when the total-pages header says so', async () => {
    const client = pagedClient(LIST_ALL_PAGE_SIZE);
    await listAll(client, '/tools');
    expect(client.get).toHaveBeenCalledTimes(1);
  });

  it('stops on a short page when the endpoint sends no paging headers', async () => {
    const total = LIST_ALL_PAGE_SIZE + 1;
    const client = pagedClient(total, { headers: false });
    const res = await listAll(client, '/tools');
    expect(res.data.data).toHaveLength(total);
    expect(client.get).toHaveBeenCalledTimes(2);
  });

  it('keeps the bare-array shape of endpoints that reply without an envelope', async () => {
    const client = pagedClient(3, { bare: true });
    const res = await listAll(client, '/filters');
    expect(Array.isArray(res.data)).toBe(true);
    expect(res.data).toHaveLength(3);
  });

  it('requests the remaining pages together rather than one after another', async () => {
    const total = LIST_ALL_PAGE_SIZE * 4;
    let inFlight = 0;
    let peak = 0;
    const base = pagedClient(total);
    const client = {
      get: jest.fn(async (...args) => {
        inFlight += 1;
        peak = Math.max(peak, inFlight);
        await new Promise((resolve) => setTimeout(resolve, 5));
        const res = await base.get(...args);
        inFlight -= 1;
        return res;
      }),
    };
    const res = await listAll(client, '/tools');
    expect(res.data.data.map((r) => r.id)).toEqual(rows(total).map((r) => r.id));
    expect(client.get).toHaveBeenCalledTimes(4);
    expect(peak).toBe(3);
  });

  it('caps the total and announces the truncation', async () => {
    const warn = jest.spyOn(console, 'warn').mockImplementation(() => {});
    const announced = jest.fn();
    const unsubscribe = subscribeListTruncated(announced);
    const client = pagedClient(LIST_ALL_MAX_ROWS + LIST_ALL_PAGE_SIZE * 3);
    const res = await listAll(client, '/users');
    expect(res.data.data).toHaveLength(LIST_ALL_MAX_ROWS);
    expect(res.truncated).toBe(true);
    expect(client.get).toHaveBeenCalledTimes(LIST_ALL_MAX_ROWS / LIST_ALL_PAGE_SIZE);
    expect(announced).toHaveBeenCalledWith({ path: '/users', limit: LIST_ALL_MAX_ROWS });
    unsubscribe();
    warn.mockRestore();
  });

  it('does not report a list of exactly the cap as truncated', async () => {
    const announced = jest.fn();
    const unsubscribe = subscribeListTruncated(announced);
    const res = await listAll(pagedClient(LIST_ALL_MAX_ROWS), '/users');
    expect(res.data.data).toHaveLength(LIST_ALL_MAX_ROWS);
    expect(res.truncated).toBe(false);
    expect(announced).not.toHaveBeenCalled();
    unsubscribe();
  });

  it('caps an endpoint without paging headers too', async () => {
    const warn = jest.spyOn(console, 'warn').mockImplementation(() => {});
    const client = pagedClient(LIST_ALL_MAX_ROWS + LIST_ALL_PAGE_SIZE * 3, { headers: false });
    const res = await listAll(client, '/users');
    expect(res.data.data).toHaveLength(LIST_ALL_MAX_ROWS);
    expect(res.truncated).toBe(true);
    expect(client.get).toHaveBeenCalledTimes(LIST_ALL_MAX_ROWS / LIST_ALL_PAGE_SIZE);
    warn.mockRestore();
  });

  it('tolerates an empty or malformed reply', async () => {
    const client = { get: jest.fn(async () => ({})) };
    const res = await listAll(client, '/tools');
    expect(res.data.data).toEqual([]);
  });
});
