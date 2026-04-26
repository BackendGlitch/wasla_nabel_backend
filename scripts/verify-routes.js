#!/usr/bin/env node

/**
 * Verify backend route wiring from Postman collection.
 *
 * Safety-first behavior:
 * - Protected routes are called with an invalid token (expecting 401).
 * - Unprotected mutating routes are sent malformed JSON to avoid side effects.
 * - Login is tested with a real CIN.
 *
 * Exit code:
 * - 0 if no FAIL
 * - 1 if one or more FAIL
 */

const fs = require('fs');
const path = require('path');

const COLORS = {
  reset: '\x1b[0m',
  red: '\x1b[31m',
  green: '\x1b[32m',
  yellow: '\x1b[33m',
  cyan: '\x1b[36m',
};

const SERVICE_META = [
  { id: 'auth', title: 'Auth Service', varName: 'auth_base_url' },
  { id: 'queue', title: 'Queue Service', varName: 'queue_base_url' },
  { id: 'booking', title: 'Booking Service', varName: 'booking_base_url' },
  { id: 'websocket', title: 'WebSocket Hub Service', varName: 'websocket_base_url' },
  { id: 'printer', title: 'Printer Service', varName: 'printer_base_url' },
  { id: 'statistics', title: 'Statistics Service', varName: 'statistics_base_url' },
];

function color(text, c) {
  return `${COLORS[c] || ''}${text}${COLORS.reset}`;
}

function parseArgs(argv) {
  const out = {
    cin: '14045739',
    collection: path.resolve(__dirname, '..', 'docs', 'Wasla_Backend.postman_collection.json'),
    timeoutMs: 8000,
    verbose: false,
  };

  for (let i = 2; i < argv.length; i += 1) {
    const a = argv[i];
    if (a === '--cin' && argv[i + 1]) {
      out.cin = argv[++i];
    } else if (a === '--collection' && argv[i + 1]) {
      out.collection = path.resolve(argv[++i]);
    } else if (a === '--timeout-ms' && argv[i + 1]) {
      out.timeoutMs = Number(argv[++i]) || out.timeoutMs;
    } else if (a === '--verbose') {
      out.verbose = true;
    } else if (a === '--help' || a === '-h') {
      out.help = true;
    }
  }

  return out;
}

function usage() {
  console.log(`Usage: node scripts/verify-routes.js [--cin 14045739] [--collection <path>] [--timeout-ms 8000] [--verbose]\n`);
}

function readCollection(filePath) {
  const raw = fs.readFileSync(filePath, 'utf8');
  return JSON.parse(raw);
}

function flattenItems(items, out = []) {
  for (const item of items || []) {
    if (item.request) out.push(item);
    if (Array.isArray(item.item)) flattenItems(item.item, out);
  }
  return out;
}

function collectionVars(collection) {
  const vars = {};
  for (const v of collection.variable || []) {
    if (typeof v?.key === 'string') vars[v.key] = String(v.value ?? '');
  }
  return vars;
}

function applyDefaultVars(vars) {
  const defaults = {
    token: '',
    staff_id: 'staff_1',
    staff_target_id: 'staff_1',
    route_id: 'route_1',
    vehicle_id: 'vehicle_1',
    auth_id: 'auth_1',
    destination_id: 'station_1',
    destination_id_2: 'station_2',
    queue_entry_id: 'queue_1',
    queue_entry_id_2: 'queue_2',
    booking_id: 'booking_1',
    station_id: 'station_1',
    printer_id: 'default',
    date: '2026-02-15',
    start_date: '2026-02-01',
    end_date: '2026-02-15',
    year: '2026',
    month: '2',
    start_rfc3339: '2026-02-15T00:00:00Z',
    end_rfc3339: '2026-02-15T23:59:59Z',
  };

  for (const [k, v] of Object.entries(defaults)) {
    if (!vars[k]) vars[k] = v;
  }
}

function applyEnvOverrides(vars) {
  const map = {
    auth_base_url: process.env.AUTH_BASE_URL,
    queue_base_url: process.env.QUEUE_BASE_URL,
    booking_base_url: process.env.BOOKING_BASE_URL,
    websocket_base_url: process.env.WEBSOCKET_BASE_URL,
    printer_base_url: process.env.PRINTER_BASE_URL,
    statistics_base_url: process.env.STATISTICS_BASE_URL,
  };

  for (const [k, v] of Object.entries(map)) {
    if (v) vars[k] = v;
  }
}

function replaceVars(str, vars) {
  if (typeof str !== 'string') return str;
  return str.replace(/\{\{\s*([^}]+?)\s*\}\}/g, (_, key) => {
    const trimmed = String(key).trim();
    return Object.prototype.hasOwnProperty.call(vars, trimmed) ? String(vars[trimmed]) : `{{${trimmed}}}`;
  });
}

function replaceVarsDeep(str, vars, maxDepth = 8) {
  if (typeof str !== 'string') return str;
  let out = str;
  for (let i = 0; i < maxDepth; i += 1) {
    const next = replaceVars(out, vars);
    if (next === out) break;
    out = next;
  }
  return out;
}

function rawUrl(req) {
  if (typeof req?.url === 'string') return req.url;
  if (typeof req?.url?.raw === 'string') return req.url.raw;
  return '';
}

function inferServiceIdFromRawUrl(raw) {
  for (const svc of SERVICE_META) {
    if (String(raw).includes(`{{${svc.varName}}}`)) return svc.id;
  }
  return null;
}

function normalizePath(url) {
  return String(url)
    .replace(/^https?:\/\/[^/]+/, '')
    .replace(/\?.*$/, '')
    .replace(/\/+/g, '/')
    .replace(/\/$/, '') || '/';
}

function isHealth(pathname) {
  return pathname === '/health';
}

function isLogin(method, pathname) {
  return method === 'POST' && pathname === '/api/v1/auth/login';
}

function isProtectedRequest(item, method, pathname) {
  const headers = item.request.header || [];
  const hasAuthHeader = headers.some((h) => String(h?.key || '').toLowerCase() === 'authorization');
  if (hasAuthHeader) return true;

  // WebSocket endpoint uses token query fallback and may not include Authorization header in one request variant.
  if (pathname.startsWith('/ws/queue/')) return true;

  return false;
}

function shouldSendMalformedBody(method, isProtected) {
  if (isProtected) return false;
  return method === 'POST' || method === 'PUT' || method === 'PATCH';
}

function shouldSkipRequest(method, pathname) {
  // This endpoint actively dials the physical printer and can block if hardware is unreachable.
  if (method === 'POST' && pathname.startsWith('/api/printer/test/')) return true;
  return false;
}

async function fetchWithTimeout(url, options, timeoutMs) {
  const controller = new AbortController();
  const id = setTimeout(() => controller.abort(), timeoutMs);
  try {
    return await fetch(url, { ...options, signal: controller.signal });
  } finally {
    clearTimeout(id);
  }
}

function summarizeExpectation({ method, pathname, isLoginReq, isHealthReq, isProtectedReq, status }) {
  if (isLoginReq) {
    if (status === 200) return { level: 'PASS', reason: 'login-ok' };
    if (status === 401) return { level: 'FAIL', reason: 'login-unauthorized (bad CIN or inactive)' };
    if (status === 400) return { level: 'FAIL', reason: 'login-bad-request' };
    if (status >= 500) return { level: 'FAIL', reason: 'login-server-error' };
    return { level: 'FAIL', reason: 'login-unexpected-status' };
  }

  if (isHealthReq) {
    if (status === 200) return { level: 'PASS', reason: 'health-ok' };
    if (status >= 500) return { level: 'FAIL', reason: 'health-server-error' };
    return { level: 'FAIL', reason: 'health-unexpected-status' };
  }

  if (status === 404 || status === 405) {
    return { level: 'FAIL', reason: 'route-missing-or-method-mismatch' };
  }

  if (isProtectedReq) {
    if (status === 401 || status === 403) {
      return { level: 'PASS', reason: 'protected-route-guard-ok' };
    }
    // Route exists but auth behavior is unexpected.
    if (status >= 500) return { level: 'WARN', reason: 'protected-route-server-error' };
    return { level: 'WARN', reason: 'protected-route-non401' };
  }

  // Unprotected routes: route exists if not 404/405.
  if (status >= 500) return { level: 'WARN', reason: 'route-exists-but-server-error' };
  if (status >= 200 && status < 500) return { level: 'PASS', reason: 'route-exists' };

  return { level: 'WARN', reason: 'unexpected-status' };
}

function levelTag(level) {
  if (level === 'PASS') return color('[PASS]', 'green');
  if (level === 'WARN') return color('[WARN]', 'yellow');
  return color('[FAIL]', 'red');
}

(async function main() {
  const args = parseArgs(process.argv);
  if (args.help) {
    usage();
    process.exit(0);
  }

  if (!fs.existsSync(args.collection)) {
    console.error(color(`Collection not found: ${args.collection}`, 'red'));
    process.exit(1);
  }

  const collection = readCollection(args.collection);
  const requests = flattenItems(collection.item || []);

  const vars = collectionVars(collection);
  applyEnvOverrides(vars);
  applyDefaultVars(vars);
  vars.staff_cin = args.cin;

  const invalidToken = 'invalid.token.value';

  let pass = 0;
  let warn = 0;
  let fail = 0;
  let skipped = 0;
  let loginToken = '';

  console.log(color('Route Verification Started', 'cyan'));
  console.log(`Collection: ${args.collection}`);
  console.log(`CIN: ${args.cin}`);
  console.log(`Requests: ${requests.length}`);
  console.log('');

  // Service preflight to avoid noisy per-route fetch failures when a service is down.
  const serviceHealth = new Map();
  for (const svc of SERVICE_META) {
    const base = String(replaceVarsDeep(String(vars[svc.varName] || ''), vars)).trim();
    if (!base) {
      serviceHealth.set(svc.id, { ok: false, base, reason: `missing var ${svc.varName}` });
      fail += 1;
      console.log(`${levelTag('FAIL')} SERVICE ${svc.title} -> ${color(`missing base URL variable: ${svc.varName}`, 'red')}`);
      continue;
    }

    const healthUrl = `${base.replace(/\/$/, '')}/health`;
    try {
      const res = await fetchWithTimeout(healthUrl, { method: 'GET' }, args.timeoutMs);
      if (res.status === 200) {
        serviceHealth.set(svc.id, { ok: true, base, reason: 'healthy' });
      } else {
        serviceHealth.set(svc.id, { ok: false, base, reason: `health status ${res.status}` });
        fail += 1;
        console.log(`${levelTag('FAIL')} SERVICE ${svc.title} -> ${color(`unreachable/invalid health at ${healthUrl} (status ${res.status})`, 'red')}`);
      }
    } catch (err) {
      const reason = err?.name === 'AbortError' ? `timeout>${args.timeoutMs}ms` : (err?.message || 'fetch failed');
      serviceHealth.set(svc.id, { ok: false, base, reason });
      fail += 1;
      console.log(`${levelTag('FAIL')} SERVICE ${svc.title} -> ${color(`cannot reach ${healthUrl} (${reason})`, 'red')}`);
    }
  }
  console.log('');

  for (const item of requests) {
    const rawTemplate = rawUrl(item.request);
    const serviceId = inferServiceIdFromRawUrl(rawTemplate);
    if (serviceId && serviceHealth.has(serviceId) && !serviceHealth.get(serviceId).ok) {
      skipped += 1;
      continue;
    }

    const method = String(item.request.method || 'GET').toUpperCase();
    const raw = replaceVarsDeep(rawUrl(item.request), vars);
    const pathname = normalizePath(raw);

    const isLoginReq = isLogin(method, pathname);
    const isHealthReq = isHealth(pathname);
    const isProtectedReq = isProtectedRequest(item, method, pathname);

    if (shouldSkipRequest(method, pathname)) {
      warn += 1;
      const name = item.name || `${method} ${pathname}`;
      console.log(`${levelTag('WARN')} ${method.padEnd(6)} ${pathname} -> ${color('(skipped-printer-hardware-endpoint)', 'cyan')} | ${name}`);
      continue;
    }

    const headers = {};
    for (const h of item.request.header || []) {
      const k = String(h?.key || '').trim();
      if (!k) continue;
      headers[k] = replaceVarsDeep(String(h?.value ?? ''), vars);
    }

    let body;

    if (isLoginReq) {
      headers['Content-Type'] = 'application/json';
      body = JSON.stringify({ cin: args.cin });
    } else if (isProtectedReq) {
      headers['Authorization'] = `Bearer ${invalidToken}`;
      // For token query fallback URL variant, also force invalid token in query.
      if (raw.includes('token=')) {
        // no-op, URL already has replaced token var; we'll replace it with invalid token explicitly.
      }
    }

    if (!isLoginReq && shouldSendMalformedBody(method, isProtectedReq)) {
      headers['Content-Type'] = headers['Content-Type'] || 'application/json';
      body = '{'; // malformed JSON to force early bind error and avoid side effects
    }

    let url = raw;
    if (url.includes('token=')) {
      url = url.replace(/token=[^&]*/g, `token=${encodeURIComponent(invalidToken)}`);
    }

    try {
      const response = await fetchWithTimeout(url, {
        method,
        headers,
        body,
      }, args.timeoutMs);

      const status = response.status;
      const rule = summarizeExpectation({ method, pathname, isLoginReq, isHealthReq, isProtectedReq, status });

      let detail = '';
      if (isLoginReq) {
        try {
          const data = await response.clone().json();
          const token = data?.data?.token;
          if (status === 200 && token) {
            loginToken = token;
            vars.token = token;
            vars.staff_id = data?.data?.staff?.id || vars.staff_id;
            vars.staff_target_id = data?.data?.staff?.id || vars.staff_target_id;
          } else if (status === 200 && !token) {
            detail = ' (token missing in response)';
            if (rule.level === 'PASS') {
              rule.level = 'FAIL';
              rule.reason = 'login-token-missing';
            }
          }
        } catch {
          if (status === 200) {
            detail = ' (non-JSON login response)';
            if (rule.level === 'PASS') {
              rule.level = 'FAIL';
              rule.reason = 'login-non-json';
            }
          }
        }
      }

      if (rule.level === 'PASS') pass += 1;
      else if (rule.level === 'WARN') warn += 1;
      else fail += 1;

      const name = item.name || `${method} ${pathname}`;
      console.log(`${levelTag(rule.level)} ${method.padEnd(6)} ${pathname} -> ${status} ${color(`(${rule.reason})`, 'cyan')} | ${name}${detail}`);

      if (args.verbose && rule.level !== 'PASS') {
        let text = '';
        try {
          text = await response.text();
        } catch {
          text = '';
        }
        if (text) {
          const t = text.length > 300 ? `${text.slice(0, 300)}...` : text;
          console.log(`       response: ${t.replace(/\n/g, ' ')}`);
        }
      }
    } catch (err) {
      fail += 1;
      const name = item.name || `${method} ${pathname}`;
      const reason = err?.name === 'AbortError' ? `timeout>${args.timeoutMs}ms` : `request-error:${err?.message || 'unknown'}`;
      console.log(`${levelTag('FAIL')} ${method.padEnd(6)} ${pathname} -> ${color(reason, 'red')} | ${name} | ${raw}`);
    }
  }

  console.log('');
  console.log(color('Summary', 'cyan'));
  console.log(`PASS: ${pass}`);
  console.log(`WARN: ${warn}`);
  console.log(`FAIL: ${fail}`);
  console.log(`SKIP: ${skipped}`);
  console.log(`Login token acquired: ${loginToken ? 'yes' : 'no'}`);

  if (fail > 0) {
    process.exit(1);
  }
})();
