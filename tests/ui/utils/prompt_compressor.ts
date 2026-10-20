import { expect, Page } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import { config } from '../config';

/**
 * Shared steps for the Prompt Compressor (community plugin) specs.
 *
 * tests/compose.yml (and the dev plugin watcher) build the plugin when the
 * community submodule has it; with an older submodule pin the specs skip.
 */

export const PLUGIN_SOURCE = path.resolve(__dirname, '../../../community/plugins/prompt-compressor/main.go');
export const PLUGIN_KEY = 'plugin:com.tyk.prompt-compressor';
export const PLUGIN_COMMAND = 'file:///app/bin/plugins/prompt-compressor';
export const SECTION = 'Prompt Compressor';

export type Json = Record<string, any>;

export function pluginAvailable(): boolean {
    return fs.existsSync(PLUGIN_SOURCE);
}

async function csrfHeaders(page: Page): Promise<Record<string, string>> {
    const origin = new URL(config.base_url).origin;
    const originHeaders = { Origin: origin, Referer: `${origin}/` };
    const csrf = await page.request.get(`${config.api_url}/csrf-token`, { headers: originHeaders });
    return { ...originHeaders, 'X-CSRF-Token': csrf.headers()['x-csrf-token'] || '' };
}

export async function api(page: Page, method: 'get' | 'post' | 'patch' | 'delete', apiPath: string, data?: Json): Promise<{ status: number; body: Json }> {
    const headers = await csrfHeaders(page);
    const res = await page.request[method](`${config.api_url}${apiPath}`, { headers, data });
    let body: Json = {};
    try { body = await res.json(); } catch { body = {}; }
    return { status: res.status(), body };
}

/** Admin RPC through Studio: the HTTP status and the plugin's own answer. */
export async function rpc(page: Page, pluginId: string, method: string, payload: Json = {}): Promise<{ status: number; data: Json }> {
    const { status, body } = await api(page, 'post', `/api/v1/plugins/${pluginId}/rpc/${method}`, payload);
    return { status, data: body && body.data !== undefined ? body.data : body };
}

export function idOf(body: Json): string {
    return String(body?.data?.id ?? body?.id ?? '');
}

export async function storedConfig(page: Page, pluginId: string): Promise<Json> {
    const { body } = await api(page, 'get', `/api/v1/plugins/${pluginId}`);
    const data = body.data ?? body;
    return data.attributes?.config ?? data.config ?? {};
}

/**
 * Registers the plugin from the dev plugin volume if it is not installed,
 * loads it, approves its scopes and waits until its RPC answers and its
 * sidebar section shows. Returns the plugin id.
 */
export async function ensurePlugin(page: Page, adminMainPage: any, readyRPC = 'getEditor'): Promise<string> {
    let pluginId = '';
    const { body } = await api(page, 'get', '/api/v1/plugins?limit=100');
    const list: Json[] = Array.isArray(body) ? body : (body.data ?? body.plugins ?? []);
    const existing = list.find((p) => (p.attributes?.command ?? p.command) === PLUGIN_COMMAND);
    if (existing) {
        pluginId = String(existing.id);
    } else {
        const res = await api(page, 'post', '/api/v1/plugins', {
            name: 'Prompt Compressor', description: 'E2E', command: PLUGIN_COMMAND,
            is_active: true, load_immediately: true, config: {},
        });
        expect(res.status, JSON.stringify(res.body)).toBeLessThan(300);
        pluginId = idOf(res.body);
    }
    expect(pluginId).not.toBe('');
    const load = await api(page, 'post', `/api/v1/plugins/${pluginId}/validate-and-load`, { command: PLUGIN_COMMAND });
    expect(load.status, JSON.stringify(load.body)).toBeLessThan(300);
    const approve = await api(page, 'post', `/api/v1/plugins/${pluginId}/approve-scopes`, { approved: true });
    expect(approve.status, JSON.stringify(approve.body)).toBeLessThan(300);
    await api(page, 'post', `/api/v1/plugins/${pluginId}/reload`, {});
    // The first session can run before the scopes are approved; retry
    await expect.poll(async () => (await rpc(page, pluginId, readyRPC)).status, { timeout: 90000, intervals: [2000] }).toBe(200);
    await expect.poll(async () => {
        await page.reload();
        await adminMainPage.dismissQuickStartModal();
        return page.getByRole('button', { name: SECTION, exact: true }).isVisible();
    }, { timeout: 90000, intervals: [2000] }).toBe(true);
    return pluginId;
}
