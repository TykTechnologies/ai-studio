import { test } from '@fixtures';
import { expect, Page } from '@playwright/test';
import * as http from 'http';
import { AddressInfo } from 'net';
import AxeBuilder from '@axe-core/playwright';
import { config } from '../config';
import { LoginPage } from '../pom/Login_page';
import { AdminMainPage } from '../pom/Admin_main_page';
import { AdminUsersPage } from '../pom/Admin_users_page';
import {
    EXFIL_HOST,
    conversationSession,
    createUserWithRole,
    csrfHeaders,
    importSession,
    largeSession,
    seedApp,
} from '../utils/agentTrace';

/**
 * Agent Call Tracing (Enterprise, jyx.35). Sessions are seeded through the
 * DEVMODE-only JSON v1 import, so the backend must run with DEVMODE=true and
 * AGENT_TRACING_ENABLED=true (see the readme).
 */

const sessionsPath = '/admin/agent-sessions';

test.skip(process.env.TEST_EDITION === 'ce', 'Enterprise only');

async function loginAsAdmin(page: Page): Promise<number> {
    const loginPage = new LoginPage(page);
    await loginPage.goto();
    await loginPage.login(config.admin_email, config.password);
    await new AdminMainPage(page).dismissQuickStartModal();
    return seedApp(page);
}

async function gotoSession(page: Page, id: string) {
    await page.goto(`${config.base_url}${sessionsPath}/${id}`);
    await expect(page.getByTestId('session-header')).toBeVisible();
}

/** Fails the test if the page ever requests the exfiltration host. */
function watchExfil(page: Page): string[] {
    const hits: string[] = [];
    page.on('request', (r) => {
        if (r.url().includes(EXFIL_HOST)) hits.push(r.url());
    });
    return hits;
}

test.describe('Agent sessions', () => {
    test('list, filters and the transcript of a tool-using turn', async ({ page }) => {
        const appId = await loginAsAdmin(page);
        const ok = conversationSession({ appId });
        const failed = conversationSession({ appId, withError: true });
        await importSession(page, ok.doc);
        await importSession(page, failed.doc);

        await page.goto(`${config.base_url}${sessionsPath}`);
        await expect(page.getByText(ok.doc.session.name)).toBeVisible();
        await expect(page.getByText(failed.doc.session.name)).toBeVisible();

        // Filters live in the URL.
        await page.goto(`${config.base_url}${sessionsPath}?errors=yes`);
        await expect(page.getByText(failed.doc.session.name)).toBeVisible();
        await expect(page.getByText(ok.doc.session.name)).toHaveCount(0);

        // Opening a row shows the transcript: user message, tool card with
        // its result, the inferred client-side tool, then the answer.
        await page.goto(`${config.base_url}${sessionsPath}`);
        await page.getByText(ok.doc.session.name).click();
        await expect(page).toHaveURL(new RegExp(`${sessionsPath}/${ok.id}`));
        const transcript = page.getByTestId('transcript');
        await expect(transcript.getByTestId('user-message').first()).toContainText('Find the forecast for Paris');
        const card = transcript.getByTestId('tool-card').filter({ hasText: 'search' });
        await expect(card).toBeVisible();
        await expect(card.getByTestId('result-preview')).toContainText('Sunny, 24 degrees');
        // The search call was matched by name and arguments (strong): it
        // carries the inferred badge, whose tooltip names the rule.
        const badge = card.getByTestId('inferred-badge');
        await expect(badge).toBeVisible();
        await badge.hover();
        await expect(page.getByRole('tooltip')).toContainText('mcp_name_args (strong confidence)');
        // The shell call ran on the client: inferred from the LLM traffic.
        await expect(transcript.getByTestId('client-side-badge')).toBeVisible();
        await expect(transcript.getByTestId('assistant-step').last()).toContainText('sunny');
    });

    test('captured Markdown never loads an image or makes a link fetch', async ({ page }) => {
        const hits = watchExfil(page);
        const appId = await loginAsAdmin(page);
        const s = conversationSession({ appId });
        await importSession(page, s.doc);
        await gotoSession(page, s.id);
        const answer = page.getByTestId('assistant-step').last();
        await expect(answer).toContainText('sunny');
        await expect(answer.getByTestId('blocked-image')).toBeVisible();
        await expect(page.locator(`img[src*="${EXFIL_HOST}"]`)).toHaveCount(0);
        const link = answer.getByRole('link', { name: /the source/ });
        await expect(link).toHaveAttribute('rel', /nofollow/);
        await expect(answer).toContainText(`https://${EXFIL_HOST}/src`);
        await page.waitForLoadState('networkidle');
        expect(hits, 'requests to the exfiltration host').toEqual([]);
    });

    test('timeline, span drawer and keyboard navigation', async ({ page }) => {
        const appId = await loginAsAdmin(page);
        const s = conversationSession({ appId });
        await importSession(page, s.doc);
        await gotoSession(page, s.id);

        // Keyboard only: 2 switches to the timeline, 1 back; n moves to the
        // next tool call, Enter opens it, Escape closes it.
        await page.keyboard.press('2');
        await expect(page.getByTestId('timeline-view')).toBeVisible();
        await page.keyboard.press('1');
        await expect(page.getByTestId('transcript')).toBeVisible();
        await page.keyboard.press('n');
        await expect(page.getByTestId('selected-marker').first()).toBeVisible();
        await page.keyboard.press('2');
        await expect(page.getByTestId('timeline-view')).toBeVisible();
        // The selection carries over to the timeline.
        await expect(page.getByTestId('timeline-view').locator('[aria-pressed="true"]')).toHaveCount(1);

        // The drawer: overview, then the captured input and the raw view.
        await page.getByTestId('timeline-view').getByTestId('timeline-bar').first().click();
        const drawer = page.getByTestId('span-drawer');
        await expect(drawer).toBeVisible();
        await expect(drawer).toContainText('gpt-4o');
        await drawer.getByRole('tab', { name: 'Input' }).click();
        await expect(drawer).toContainText('Find the forecast for Paris');
        // Pretty by default; the JSON toggle shows the content as captured.
        await drawer.getByRole('button', { name: 'JSON' }).first().click();
        await expect(drawer.getByTestId('raw-view').first()).toContainText('"messages"');
        await drawer.getByRole('button', { name: 'Pretty' }).first().click();
        await page.keyboard.press('Escape');
        await expect(drawer).toBeHidden();
    });

    test('dragging a range on the timeline strip filters the transcript', async ({ page }) => {
        const appId = await loginAsAdmin(page);
        const s = conversationSession({ appId });
        await importSession(page, s.doc);
        await gotoSession(page, s.id);
        const transcript = page.getByTestId('transcript');
        await expect(transcript).toContainText('It is');

        // The session spans 2.4 s; the first quarter holds only the first
        // LLM call, not the final answer at 1.6 s.
        const track = page.getByTestId('timeline-strip-track');
        const box = (await track.boundingBox())!;
        const y = box.y + box.height / 2;
        await page.mouse.move(box.x + 2, y);
        await page.mouse.down();
        await page.mouse.move(box.x + box.width * 0.25, y, { steps: 5 });
        await page.mouse.up();
        await expect(page.getByTestId('range-label')).toBeVisible();
        await expect(transcript).not.toContainText('It is');
        await expect(transcript.getByTestId('assistant-step').first()).toBeVisible();

        await page.getByRole('button', { name: 'Clear range' }).click();
        await expect(page.getByTestId('range-label')).toBeHidden();
        await expect(transcript).toContainText('It is');
    });

    test('exports match the API', async ({ page }) => {
        const appId = await loginAsAdmin(page);
        const s = conversationSession({ appId });
        await importSession(page, s.doc);
        await gotoSession(page, s.id);

        await page.getByRole('button', { name: 'Export' }).click();
        const [json] = await Promise.all([page.waitForEvent('download'), page.getByRole('menuitem', { name: 'JSON (metadata)' }).click()]);
        const fromUI = JSON.parse(await streamText(await json.createReadStream()));
        const api = await page.request.get(`${config.api_url}/api/v1/agent-tracing/sessions/${s.id}/export?format=json&content=false`);
        expect(api.ok()).toBeTruthy();
        const fromAPI = await api.json();
        delete fromUI.exported_at;
        delete fromAPI.exported_at;
        expect(fromUI).toEqual(fromAPI);
        expect(fromUI.session.id).toBe(s.id);

        await page.getByRole('button', { name: 'Export' }).click();
        const [md] = await Promise.all([page.waitForEvent('download'), page.getByRole('menuitem', { name: 'Markdown transcript' }).click()]);
        const mdUI = stripExportTime(await streamText(await md.createReadStream()));
        const mdAPI = await page.request.get(`${config.api_url}/api/v1/agent-tracing/sessions/${s.id}/export?format=markdown&content=true`);
        expect(stripExportTime(await mdAPI.text())).toBe(mdUI);
        expect(mdUI).toContain(s.doc.session.name);
    });

    test('a 5,000-span session renders within budget', async ({ page }) => {
        test.setTimeout(180_000);
        const appId = await loginAsAdmin(page);
        const big = largeSession(5000, appId);
        await importSession(page, big.doc);

        // Measure the in-app navigation only: from the click on the session in
        // the list (the app is already loaded and logged in) to the first
        // transcript row in the DOM. That is the session fetch plus the
        // render. Timing from page load, as before, also counted the bundle,
        // the app bootstrap and the login, and failed on slow CI runners.
        await page.goto(`${config.base_url}${sessionsPath}`);
        const entry = page.getByText(big.doc.session.name);
        await expect(entry).toBeVisible();
        // A MutationObserver marks the first row as it is added, so the time
        // does not include Playwright's polling interval.
        await page.evaluate(() => {
            new MutationObserver((_, observer) => {
                if (document.querySelector('[data-testid="transcript-row"]')) {
                    performance.mark('agent-session-first-row');
                    observer.disconnect();
                }
            }).observe(document.body, { childList: true, subtree: true });
            performance.mark('agent-session-open');
        });
        await entry.click();
        await expect(page.getByTestId('transcript-row').first()).toBeVisible({ timeout: 10_000 });
        const ms = await page.evaluate(
            () => performance.measure('agent-session-render', 'agent-session-open', 'agent-session-first-row').duration,
        );
        console.log(`5,000-span session: first transcript row ${Math.round(ms)} ms after the click`);
        // The time still includes fetching the 5,000 spans, so the budget
        // leaves room for a slow CI runner. A non-virtualised render of 5,000
        // rows takes several seconds, and the row count below catches it
        // whatever the timing.
        expect(ms).toBeLessThanOrEqual(3000);
        // The list is virtualised: far fewer rows in the DOM than steps.
        expect(await page.getByTestId('transcript-row').count()).toBeLessThan(200);
    });

    test('accessibility: no serious axe violations on the list and a session', async ({ page }) => {
        const appId = await loginAsAdmin(page);
        const s = conversationSession({ appId });
        await importSession(page, s.doc);

        await page.goto(`${config.base_url}${sessionsPath}`);
        await expect(page.getByText(s.doc.session.name)).toBeVisible();
        await expectNoSeriousViolations(page, '[data-testid="session-filters"]');

        await gotoSession(page, s.id);
        await expect(page.getByTestId('tool-card').first()).toBeVisible();
        await expectNoSeriousViolations(page, '[data-testid="session-header"]');
        await expectNoSeriousViolations(page, '[data-testid="transcript"]');
        await page.keyboard.press('2');
        await expectNoSeriousViolations(page, '[data-testid="timeline-view"]');
    });
});

test.describe('Agent sessions: content permission', () => {
    for (const role of ['Viewer', 'Auditor']) {
        test(`${role} sees metadata only, an admin sees content and is audited`, async ({ page }) => {
            const appId = await loginAsAdmin(page);
            const s = conversationSession({ appId });
            await importSession(page, s.doc);
            const email = `${role.toLowerCase()}_${Date.now()}@tyk.io`;
            await createUserWithRole(page, role, email, config.password);

            // The administrator reads content; the read is in the audit trail.
            await gotoSession(page, s.id);
            await expect(page.getByTestId('result-preview').first()).toContainText('Sunny');
            await expect
                .poll(async () => {
                    const audit = await page.request.get(
                        `${config.api_url}/api/v1/audit/records?resource_type=agent_trace_session&action=${encodeURIComponent('View Agent Trace Content')}`
                    );
                    return audit.ok() ? JSON.stringify(await audit.json()) : `HTTP ${audit.status()}`;
                })
                .toContain(s.id);

            await new AdminUsersPage(page).logOut();
            await page.waitForLoadState('networkidle');
            const loginPage = new LoginPage(page);
            await loginPage.goto();
            await loginPage.login(email, config.password);
            await new AdminMainPage(page).dismissQuickStartModal();

            await gotoSession(page, s.id);
            await expect(page.getByText('Your role shows the structure of this session but not its content.')).toBeVisible();
            await expect(page.getByTestId('tool-card').first()).toBeVisible();
            await expect(page.getByTestId('content-placeholder').first()).toBeVisible();
            await expect(page.getByTestId('blocked-image')).toHaveCount(0);
            await expect(page.getByText('Sunny, 24 degrees')).toHaveCount(0);
            await expect(page.getByText(s.secret)).toHaveCount(0);
            const payload = await page.request.get(
                `${config.api_url}/api/v1/agent-tracing/sessions/${s.id}/spans?include=previews`
            );
            expect(await payload.text()).not.toContain('Sunny');
        });
    }
});

async function expectNoSeriousViolations(page: Page, selector: string) {
    const results = await new AxeBuilder({ page }).include(selector).analyze();
    const serious = results.violations.filter((v) => v.impact === 'serious' || v.impact === 'critical');
    expect(serious.map((v) => `${v.id}: ${v.help} (${v.nodes.map((n) => `${n.target.join(' ')} ${n.html.slice(0, 160)}`).join(' | ')})`)).toEqual([]);
}

async function streamText(stream: NodeJS.ReadableStream): Promise<string> {
    const chunks: Buffer[] = [];
    for await (const c of stream) chunks.push(Buffer.from(c as Buffer));
    return Buffer.concat(chunks).toString('utf8');
}

const stripExportTime = (md: string) =>
    md
        .split('\n')
        .filter((l) => !/exported/i.test(l))
        .join('\n');

/**
 * Live capture (security's case): a request through the gateway carrying
 * an email address and an OpenAI key, to an App whose content mode is
 * redacted. The transcript and the export show the redaction markers, and
 * the raw values appear nowhere. Needs AGENT_TRACING_CONTENT_DEFAULT_MODE
 * at least redacted (an App can only lower it), ALLOW_INTERNAL_NETWORK_ACCESS
 * for the in-test mock upstream, and TEST_PROXY_URL (the gateway port).
 * When Studio runs in a container, TEST_MOCK_UPSTREAM_HOST names this
 * machine as the container sees it (host.docker.internal in CI).
 */
test.describe('Agent sessions: live capture', () => {
    test.skip(!process.env.TEST_PROXY_URL, 'set TEST_PROXY_URL to the embedded gateway (PROXY_PORT)');

    test('redacted content mode strips secrets and personal data', async ({ page }) => {
        test.setTimeout(150_000);
        const email = `jane.doe.${Date.now()}@example.com`;
        const apiKey = `sk-proj-${'A1b2C3d4E5f6G7h8'.repeat(3)}`;
        const upstream = http.createServer((req, res) => {
            req.resume();
            req.on('end', () => {
                res.setHeader('Content-Type', 'application/json');
                res.end(JSON.stringify({
                    id: 'chatcmpl-e2e', object: 'chat.completion', created: Math.floor(Date.now() / 1000), model: 'gpt-4o',
                    choices: [{ index: 0, finish_reason: 'stop', message: { role: 'assistant', content: 'Noted, I will contact them.' } }],
                    usage: { prompt_tokens: 20, completion_tokens: 6, total_tokens: 26 },
                }));
            });
        });
        const upstreamHost = process.env.TEST_MOCK_UPSTREAM_HOST || '127.0.0.1';
        // A containerised Studio reaches the mock through the host gateway.
        const bindHost = upstreamHost === '127.0.0.1' ? '127.0.0.1' : '0.0.0.0';
        await new Promise<void>((r) => upstream.listen(0, bindHost, () => r()));
        const port = (upstream.address() as AddressInfo).port;
        try {
            const appId = await loginAsAdmin(page);
            const h = { ...(await csrfHeaders(page)), 'Content-Type': 'application/json' };
            const api = (path: string) => `${config.api_url}/api/v1${path}`;
            const name = `e2e tracing llm ${Date.now()}`;
            const llm = await page.request.post(api('/llms'), { headers: h, data: { data: { type: 'llms', attributes: {
                name, vendor: 'openai', api_endpoint: `http://${upstreamHost}:${port}/v1`, api_key: 'sk-upstream-e2e',
                default_model: 'gpt-4o', active: true, privacy_score: 50, allowed_models: [],
            } } } });
            expect(llm.status(), await llm.text()).toBeLessThan(300);
            const llmBody = (await llm.json()).data;
            const app = await page.request.post(api('/apps'), { headers: h, data: { data: { type: 'apps', attributes: {
                name: `e2e tracing app ${Date.now()}`, description: 'agent tracing e2e', user_id: 1,
                llm_ids: [Number(llmBody.id)], tracing: { content_mode: 'redacted' },
            } } } });
            expect(app.status(), await app.text()).toBeLessThan(300);
            const appBody = (await app.json()).data;
            const activate = await page.request.post(api(`/apps/${appBody.id}/activate-credential`), { headers: h });
            expect(activate.status(), await activate.text()).toBeLessThan(300);
            const cred = await page.request.get(api(`/credentials/${appBody.attributes.credential_id}`));
            expect(cred.ok(), await cred.text()).toBeTruthy();
            const secret = (await cred.json()).data.attributes.secret;

            const slug = llmBody.attributes.slug || name.toLowerCase().replace(/[^a-z0-9]+/g, '-');
            // The gateway picks up the new App and credential on its next
            // configuration reload.
            await expect
                .poll(async () => {
                    const call = await page.request.post(`${process.env.TEST_PROXY_URL}/llm/rest/${slug}/v1/chat/completions`, {
                        headers: { Authorization: `Bearer ${secret}`, 'Content-Type': 'application/json' },
                        data: { model: 'gpt-4o', messages: [{ role: 'user', content: `Email ${email} the new key ${apiKey} please.` }] },
                    });
                    return `${call.status()} ${call.status() === 200 ? '' : await call.text()}`;
                }, { timeout: 60_000, intervals: [2_000] })
                .toMatch(/^200 /);

            // The assembler waits out its settle window (30 s) before the
            // session appears.
            let sessionId = '';
            await expect
                .poll(async () => {
                    const r = await page.request.get(api(`/agent-tracing/sessions?app_id=${appBody.id}&range=24h`));
                    const body = await r.json();
                    const rows = body.data || body.sessions || [];
                    sessionId = rows[0]?.id || '';
                    return sessionId;
                }, { timeout: 90_000, intervals: [2_000] })
                .not.toBe('');

            await gotoSession(page, sessionId);
            const transcript = page.getByTestId('transcript');
            await expect(transcript.getByTestId('user-message').first()).toContainText('REDACTED');
            await expect(transcript).toContainText('EMAIL');
            await expect(page.getByText(email)).toHaveCount(0);
            await expect(page.getByText(apiKey)).toHaveCount(0);
            const exp = await page.request.get(api(`/agent-tracing/sessions/${sessionId}/export?format=json&content=true`));
            const text = await exp.text();
            expect(exp.ok(), text).toBeTruthy();
            expect(text).toContain('REDACTED');
            expect(text).not.toContain(email);
            expect(text).not.toContain(apiKey);
        } finally {
            upstream.close();
        }
    });
});
