#!/usr/bin/env node

const fs = require('fs');
const path = require('path');
const { SERVICE_SPECS, extractRoutes } = require('./lib/route-extractor');

const repoRoot = path.resolve(__dirname, '..');
const outFile = path.resolve(repoRoot, 'docs', 'API_ROUTES.generated.md');

function rel(p) {
  return path.relative(repoRoot, p).replace(/\\/g, '/');
}

function groupRoutes(routes) {
  const byService = new Map();
  for (const spec of SERVICE_SPECS) byService.set(spec.id, []);
  for (const r of routes) byService.get(r.service)?.push(r);
  for (const [k, list] of byService.entries()) {
    byService.set(k, list.sort((a, b) => {
      if (a.path !== b.path) return a.path.localeCompare(b.path);
      return a.method.localeCompare(b.method);
    }));
  }
  return byService;
}

function renderMarkdown(routes) {
  const byService = groupRoutes(routes);

  const lines = [];
  lines.push('# API Routes (Generated)');
  lines.push('');
  lines.push('This file is auto-generated from route declarations in Go source code.');
  lines.push('Do not edit manually. Run `node scripts/sync-api-route-matrix.js`.');
  lines.push('');
  lines.push(`Total routes: **${routes.length}**`);
  lines.push('');

  for (const spec of SERVICE_SPECS) {
    const list = byService.get(spec.id) || [];
    lines.push(`## ${spec.title}`);
    lines.push('');
    lines.push(`Base URL: \`${spec.defaultBaseUrl}\``);
    lines.push('');
    lines.push('| Method | Path | Auth | Source |');
    lines.push('|---|---|---|---|');
    for (const r of list) {
      lines.push(`| ${r.method} | \`${r.path}\` | ${r.auth ? 'Yes' : 'No'} | \`${rel(r.sourceFile)}:${r.sourceLine}\` |`);
    }
    lines.push('');
  }

  return `${lines.join('\n')}\n`;
}

function main() {
  const routes = extractRoutes(repoRoot);
  const content = renderMarkdown(routes);
  fs.mkdirSync(path.dirname(outFile), { recursive: true });
  fs.writeFileSync(outFile, content, 'utf8');
  console.log(`Wrote ${outFile}`);
  console.log(`Routes: ${routes.length}`);
}

main();
