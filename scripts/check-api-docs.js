#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const { SERVICE_SPECS, extractRoutes, normalizePath } = require('./lib/route-extractor');

const repoRoot = path.resolve(__dirname, '..');

const DOC_SERVICE_TITLES = {
  'Auth Service (`http://localhost:8001`)': 'auth',
  'Queue Service (`http://localhost:8002`)': 'queue',
  'Booking Service (`http://localhost:8003`)': 'booking',
  'Printer Service (`http://localhost:8005`)': 'printer',
  'Statistics Service (`http://localhost:8006`)': 'statistics',
  'WebSocket Hub Service (`http://localhost:8004`)': 'websocket',
  'Public Service (`http://localhost:8007`)': 'public',
};

function canonicalPath(p) {
  return normalizePath(
    String(p || '')
      .replace(/^https?:\/\/[^/]+/, '')
      .replace(/^\{\{[^}]+\}\}/, '')
      .replace(/\?.*$/, '')
      .replace(/\{\{\s*[^}]+\s*\}\}/g, ':id')
      .replace(/:[A-Za-z_][A-Za-z0-9_]*/g, ':id'),
  );
}

function setFromRoutes(routes) {
  const s = new Set();
  for (const r of routes) {
    s.add(`${r.service}|${r.method}|${canonicalPath(r.path)}`);
  }
  return s;
}

function parseMarkdownRoutes(filePath) {
  const content = fs.readFileSync(filePath, 'utf8');
  const lines = content.split(/\r?\n/);

  const out = new Set();
  let currentService = null;

  for (const line of lines) {
    const heading = line.match(/^##\s+(.+)\s*$/);
    if (heading) {
      currentService = DOC_SERVICE_TITLES[heading[1]] || null;
      continue;
    }

    const m = line.match(/^\|\s*(GET|POST|PUT|PATCH|DELETE|OPTIONS|ANY)\s*\|\s*`([^`]+)`\s*\|/i);
    if (!m || !currentService) continue;

    const method = m[1].toUpperCase();
    const routePath = canonicalPath(m[2]);
    out.add(`${currentService}|${method}|${routePath}`);
  }

  return out;
}

function inferServiceFromUrl(rawUrl) {
  for (const spec of SERVICE_SPECS) {
    if (rawUrl.includes(`{{${spec.baseUrlVar}}}`)) return spec.id;
  }
  return null;
}

function parseCollectionRoutes(filePath) {
  const collection = JSON.parse(fs.readFileSync(filePath, 'utf8'));

  function flatten(items, out = []) {
    for (const item of items || []) {
      if (item.request) out.push(item);
      if (Array.isArray(item.item)) flatten(item.item, out);
    }
    return out;
  }

  const reqs = flatten(collection.item || []);
  const out = new Set();

  for (const reqItem of reqs) {
    const method = String(reqItem.request?.method || '').toUpperCase();
    if (!method) continue;

    const raw = typeof reqItem.request.url === 'string'
      ? reqItem.request.url
      : (reqItem.request.url?.raw || '');

    const service = inferServiceFromUrl(raw);
    if (!service) continue;

    out.add(`${service}|${method}|${canonicalPath(raw)}`);
  }

  return out;
}

function diff(a, b) {
  const out = [];
  for (const x of a) if (!b.has(x)) out.push(x);
  return out.sort();
}

function pretty(label, arr) {
  console.log(`${label}: ${arr.length}`);
  if (!arr.length) return;
  for (const x of arr) console.log(`  - ${x}`);
}

function main() {
  const codeRoutes = extractRoutes(repoRoot);
  const codeSet = setFromRoutes(codeRoutes);

  const markdownFile = path.resolve(repoRoot, 'docs', 'API_DOCUMENTATION.md');
  const postmanFile = path.resolve(repoRoot, 'docs', 'Wasla_Backend.postman_collection.json');

  const mdSet = parseMarkdownRoutes(markdownFile);
  const pmSet = parseCollectionRoutes(postmanFile);

  const missingInMd = diff(codeSet, mdSet);
  const missingInPm = diff(codeSet, pmSet);
  const extraInMd = diff(mdSet, codeSet);
  const extraInPm = diff(pmSet, codeSet);

  console.log('Code routes:', codeSet.size);
  console.log('Markdown routes:', mdSet.size);
  console.log('Postman routes:', pmSet.size);
  console.log('');

  pretty('Missing in API_DOCUMENTATION.md', missingInMd);
  pretty('Missing in Postman collection', missingInPm);
  pretty('Extra in API_DOCUMENTATION.md', extraInMd);
  pretty('Extra in Postman collection', extraInPm);

  const failed = missingInMd.length || missingInPm.length || extraInMd.length || extraInPm.length;
  if (failed) {
    process.exit(1);
  }

  console.log('\nDocs and Postman route coverage match code routes.');
}

main();
