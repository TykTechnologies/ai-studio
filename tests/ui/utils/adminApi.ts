import { APIResponse, Page, expect } from '@playwright/test';
import { config } from '../config';

/**
 * Small admin API helpers for specs that set up and clean up their own
 * fixtures through the browser's logged-in session (page.request). State-
 * changing calls need the CSRF header plus Origin/Referer, which
 * page.request does not send by itself.
 */
export async function csrfHeaders(page: Page): Promise<Record<string, string>> {
    const origin = new URL(config.base_url).origin;
    const originHeaders = { Origin: origin, Referer: `${origin}/` };
    const csrf = await page.request.get(`${config.api_url}/csrf-token`, { headers: originHeaders });
    return { ...originHeaders, 'X-CSRF-Token': csrf.headers()['x-csrf-token'] || '' };
}

export async function apiCall(
    page: Page,
    method: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE',
    path: string,
    data?: unknown,
): Promise<APIResponse> {
    const url = `${config.api_url}${path}`;
    if (method === 'GET') {
        return page.request.get(url);
    }
    const headers = await csrfHeaders(page);
    return page.request.fetch(url, { method, headers, data });
}

/** Calls the API and fails the test with the response body on an unexpected status. */
export async function apiExpect(
    page: Page,
    method: 'GET' | 'POST' | 'PATCH' | 'PUT' | 'DELETE',
    path: string,
    status: number,
    data?: unknown,
): Promise<any> {
    const response = await apiCall(page, method, path, data);
    const text = await response.text();
    expect(response.status(), `${method} ${path}: ${text.slice(0, 300)}`).toBe(status);
    return text ? JSON.parse(text) : undefined;
}

/** Best-effort delete for cleanup: never throws, so one failure does not hide the next. */
export async function apiDeleteQuietly(page: Page, path: string): Promise<void> {
    try {
        await apiCall(page, 'DELETE', path);
    } catch {
        // cleanup only
    }
}

/** The id of a system or custom role by name. */
export async function roleId(page: Page, name: string): Promise<number> {
    const body = await apiExpect(page, 'GET', '/api/v1/rbac/roles', 200);
    const role = (body.data || []).find((r: any) => r.attributes?.name === name);
    expect(role, `role ${name}`).toBeTruthy();
    return Number(role.id);
}

/** Creates a user with a password and no roles; returns its id. */
export async function createUser(page: Page, email: string, name: string, password: string): Promise<number> {
    const body = await apiExpect(page, 'POST', '/api/v1/users', 201, {
        data: { type: 'users', attributes: { email, name, password, email_verified: true } },
    });
    return Number(body.data.id);
}

/** Looks a Workspace up by name; undefined when there is none. */
export async function workspaceIdByName(page: Page, name: string): Promise<number | undefined> {
    const body = await apiExpect(page, 'GET', '/api/v1/workspaces', 200);
    const found = (body.data || []).find((w: any) => w.attributes?.name === name);
    return found ? Number(found.id) : undefined;
}

/** Deletes every role binding of a user (any scope). */
export async function deleteUserBindings(page: Page, userId: number): Promise<void> {
    const response = await apiCall(page, 'GET', `/api/v1/rbac/bindings?subject_type=user&subject_id=${userId}`);
    if (!response.ok()) {
        return;
    }
    const body = await response.json();
    for (const b of body.data || []) {
        await apiDeleteQuietly(page, `/api/v1/rbac/bindings/${b.id}`);
    }
}
