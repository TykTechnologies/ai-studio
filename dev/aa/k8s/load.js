// k6 load for the two-site stack on Kubernetes (dev/aa/k8s). One run is one
// fault: steady load for the whole DURATION while aa-k8s.sh injects it.
//
//   proxied   chat completions through each edge with an App key the edge
//             validated before the fault            EDGE_RPS per edge
//   write     create and delete a group through each Studio   WRITE_RPS each
//   login     a login through each Studio                     LOGIN_RPS each
//
// Every scenario has a user of its own: Studio keeps one session per user, so
// two scenarios sharing one would log each other out.
// Every response that is not 2xx is printed as one JSON line (time, scenario,
// status, error code), which is what recovery times are read from.
import http from 'k6/http';
import { Counter } from 'k6/metrics';

const env = (k, d) => (__ENV[k] === undefined || __ENV[k] === '' ? d : __ENV[k]);
const DURATION = env('DURATION', '10m');
const EDGE_RPS = Number(env('EDGE_RPS', '100'));
const WRITE_RPS = Number(env('WRITE_RPS', '1'));
const LOGIN_RPS = Number(env('LOGIN_RPS', '0.5'));
const ORIGIN = env('ORIGIN', 'http://studio.aa-load.test');
const ROUTE = env('ROUTE', 'mock-fast');
const MODEL = env('MODEL', 'mock-model');
const PASSWORD = __ENV.PROBE_PASSWORD;

const sites = {
  a: { edge: env('EDGE_A', 'http://edge-a-microgateway:8080'), studio: env('STUDIO_A', 'http://studio-a:8080') },
  b: { edge: env('EDGE_B', 'http://edge-b-microgateway:8080'), studio: env('STUDIO_B', 'http://studio-b:8080') },
};

const rate = (perSecond, exec, envs, vus) => {
  // constant-arrival-rate takes whole numbers: 0.5/s is 1 per 2 s.
  const unit = perSecond >= 1 ? 1 : Math.round(1 / perSecond);
  return {
    executor: 'constant-arrival-rate', exec, env: envs, duration: DURATION,
    rate: perSecond >= 1 ? Math.round(perSecond) : 1, timeUnit: `${unit}s`,
    preAllocatedVUs: vus, maxVUs: vus * 4, gracefulStop: '35s',
  };
};

const scenarios = {};
const thresholds = {};
for (const s of ['a', 'b']) {
  scenarios[`proxied_${s}`] = rate(EDGE_RPS, 'proxied', { SITE: s }, Math.max(20, EDGE_RPS));
  scenarios[`write_${s}`] = rate(WRITE_RPS, 'write', { SITE: s, USER: `aa-probe-write-${s}@tyk.io` }, 4);
  scenarios[`login_${s}`] = rate(LOGIN_RPS, 'login', { SITE: s, USER: `aa-probe-login-${s}@tyk.io` }, 4);
  // A threshold per scenario only so that the summary breaks the numbers
  // down by scenario; none of them can fail the run.
  for (const k of ['proxied', 'write', 'login']) {
    thresholds[`http_req_duration{scenario:${k}_${s}}`] = ['p(99)>=0'];
    thresholds[`http_reqs{scenario:${k}_${s}}`] = ['count>=0'];
    thresholds[`not_2xx{scenario:${k}_${s}}`] = ['count>=0'];
  }
}

export const options = {
  scenarios, thresholds,
  summaryTrendStats: ['count', 'avg', 'p(50)', 'p(95)', 'p(99)', 'max'],
  // A fault must show as failed requests, not as a stalled generator.
  noConnectionReuse: false,
};

const not2xx = new Counter('not_2xx');

function note(res, what) {
  if (res.status >= 200 && res.status < 300) return true;
  let code = '';
  try { const b = res.json(); code = (b.error && (b.error.code || b.error)) || b.code || ''; } catch (e) { code = String(res.error || '').slice(0, 80); }
  not2xx.add(1, { status: String(res.status) });
  console.log(JSON.stringify({ t: new Date().toISOString(), scenario: __ENV.SITE ? `${what}_${__ENV.SITE}` : what, status: res.status, code: String(code).slice(0, 120), ms: Math.round(res.timings.duration) }));
  return false;
}

export function proxied() {
  const res = http.post(`${sites[__ENV.SITE].edge}/llm/rest/${ROUTE}/v1/chat/completions`,
    JSON.stringify({ model: MODEL, messages: [{ role: 'user', content: 'ping' }] }),
    { headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${__ENV.APP_KEY}` }, timeout: '30s', tags: { kind: 'proxied' } });
  note(res, 'proxied');
}

function csrf(studio) {
  const res = http.get(`${studio}/csrf-token`, { headers: { Origin: ORIGIN }, tags: { kind: 'csrf' }, timeout: '30s' });
  return res.headers['X-Csrf-Token'] || '';
}

function headers(token) {
  return { 'Content-Type': 'application/json', Origin: ORIGIN, Referer: `${ORIGIN}/`, 'X-CSRF-Token': token };
}

function doLogin(studio, kind) {
  const res = http.post(`${studio}/auth/login`,
    JSON.stringify({ data: { type: 'user', attributes: { email: __ENV.USER, password: PASSWORD } } }),
    { headers: headers(csrf(studio)), tags: { kind }, timeout: '30s' });
  return note(res, kind);
}

export function login() {
  doLogin(sites[__ENV.SITE].studio, 'login');
}

let loggedIn = false; // per VU; the cookie jar is per VU too
export function write() {
  const studio = sites[__ENV.SITE].studio;
  if (!loggedIn) loggedIn = doLogin(studio, 'write-login');
  if (!loggedIn) return;
  const name = `aa-load-${__ENV.SITE}-${__VU}-${__ITER}-${Date.now()}`;
  const created = http.post(`${studio}/api/v1/groups`,
    JSON.stringify({ data: { type: 'group', attributes: { name, members: [], catalogues: [], data_catalogues: [], tool_catalogues: [] } } }),
    { headers: headers(csrf(studio)), tags: { kind: 'write' }, timeout: '60s' });
  if (!note(created, 'write')) { if (created.status === 401) loggedIn = false; return; }
  let id = '';
  try { id = created.json().data.id; } catch (e) { return; }
  const deleted = http.del(`${studio}/api/v1/groups/${id}`, null, { headers: headers(csrf(studio)), tags: { kind: 'write' }, timeout: '60s' });
  if (!note(deleted, 'write') && deleted.status === 401) loggedIn = false;
}
